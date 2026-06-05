package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// jdKeyRe matches valid slot keys: jd_ followed by 1-3 digits (Perl: m/jd_\d{1,3}$/)
var jdKeyRe = regexp.MustCompile(`^jd_\d{1,3}$`)

// execFr implements the fr/f command (matches Perl exec_fr_process).
func execFr(cfg Config, args []string, password string, outFile string) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()

	checkVersion(rdb)

	var slotNum string

	if len(args) > 0 {
		arg := args[0]
		// Must match jd_\d{1,3} exactly, matching Perl: m/${JD_PREFIX}\d{1,3}$/
		if !jdKeyRe.MatchString(arg) {
			currentSlot := getCurrentSlotNum(rdb)
			fmt.Fprintf(os.Stderr, "- argv should be jd_xx, less than jd_%s\n", currentSlot)
			os.Exit(1)
		}
		slotNum = strings.TrimPrefix(arg, JD_SLOT_PREFIX)
	} else {
		// No arg: GET jd_incr directly (matches Perl get_jd_xx_from_incr)
		slotNum = getCurrentSlotNum(rdb)
	}

	slotKey := fmt.Sprintf("%s%s", JD_SLOT_PREFIX, slotNum)
	pwKey := fmt.Sprintf("%s%s", PW_PREFIX, slotNum)

	// Password check: up to 3 interactive attempts; only 1 if -pw was given
	storedPw, err := rdb.Get(ctx, pwKey).Result()
	if err == nil && storedPw != "" {
		const maxAttempts = 3
		ok := false
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			pw := password
			if pw == "" {
				pw = readPassword("- need password, please input: ")
			}
			inputCrc := fmt.Sprintf("%d", mycrc32([]byte(pw)))
			if inputCrc == strings.TrimSpace(storedPw) {
				ok = true
				break
			}
			if password != "" {
				fmt.Fprintln(os.Stderr, "ERR: wrong password")
				os.Exit(1)
			}
			if attempt < maxAttempts {
				fmt.Fprintf(os.Stderr, "wrong password, try again (%d/%d)\n", attempt, maxAttempts)
			}
		}
		if !ok {
			fmt.Fprintln(os.Stderr, "ERR: wrong password")
			os.Exit(1)
		}
	}

	// ── Early overwrite check (Perl does this BEFORE downloading data) ──────
	// Read filename from the separate FILENAME_:jd_XX key — O(1), fast.
	fnKey := FNAME_PREFIX + slotNum
	storedFn, _ := rdb.Get(ctx, fnKey).Result()
	if storedFn == "" {
		storedFn = "txt.txt"
	}
	// Check if storedFn itself exists on disk (matches Perl gen_overwrite_struct hash_0).
	// This catches leftover .tar.gz / files from failed prior transfers.
	if storedFn != "txt.txt" && !promptOverwrite(storedFn) {
		os.Exit(1)
	}
	earlyIsFolder := strings.HasPrefix(storedFn, "FOLDER_") && strings.HasSuffix(storedFn, ".tar.gz")
	if earlyIsFolder {
		folderName := strings.TrimSuffix(strings.TrimPrefix(storedFn, "FOLDER_"), ".tar.gz")
		if !promptOverwrite(folderName) {
			os.Exit(1)
		}
	} else if storedFn != "txt.txt" {
		saveName := storedFn
		if outFile != "" {
			saveName = outFile
		}
		if !promptOverwrite(saveName) {
			os.Exit(1)
		}
	}

	// Get key size for accurate progress bar (actual bytes on wire).
	sizeBytes, _ := rdb.StrLen(ctx, slotKey).Result()
	if sizeBytes == 0 {
		// Key does not exist (matches Perl: "- should res_raw_text not NULL")
		fmt.Fprintf(os.Stderr, "- slot data not found or already cleared: %s\n", slotKey)
		os.Exit(1)
	}

	startTime := time.Now()
	var raw []byte
	var getErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		var bytesRead int64
		largeCli := newLargeOpClient(cfg, &bytesRead, nil)
		runWithBytesProgress(sizeBytes, &bytesRead, func() {
			raw, getErr = largeCli.Get(ctx, slotKey).Bytes()
		})
		largeCli.Close()
		if getErr == nil {
			break
		}
		// redis.Nil means key doesn't exist — fail immediately, don't retry
		if getErr == redis.Nil {
			fmt.Fprintf(os.Stderr, "\n- slot data not found or already cleared: %s\n", slotKey)
			os.Exit(1)
		}
		// Other errors: retry (network transient, timeout, etc.)
		if attempt < maxRetries {
			fmt.Fprintf(os.Stderr, "\n- download error, retrying in 4s... (%d/%d): %v\n", attempt, maxRetries, getErr)
			time.Sleep(retryDelay)
		} else {
			fmt.Fprintf(os.Stderr, "\nERR: redis GET %s: %v\n", slotKey, getErr)
			os.Exit(1)
		}
	}

	isGzip, filename, storedCrc, payload, err := parseMetaHeader(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: parse header: %v\n", err)
		os.Exit(1)
	}

	var finalData []byte
	if isGzip {
		finalData, err = gunzipBytes(payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR: gunzip: %v\n", err)
			os.Exit(1)
		}
	} else {
		finalData = payload
	}

	computedCrc := mycrc32(finalData)
	if computedCrc != storedCrc {
		fmt.Fprintf(os.Stderr, "ERR: CRC32 mismatch: got %d, want %d\n", computedCrc, storedCrc)
		os.Exit(1)
	}

	isFolder := strings.HasPrefix(filename, "FOLDER_") && strings.HasSuffix(filename, ".tar.gz")
	cost := time.Since(startTime).Seconds()
	ip := getClientIP(rdb)
	slotInt := parseSlotInt(slotNum)

	if isFolder {
		folderName := strings.TrimSuffix(strings.TrimPrefix(filename, "FOLDER_"), ".tar.gz")
		destDir := "."
		if outFile != "" {
			destDir = outFile
		}
		// Second overwrite check (matches Perl do_untar_to_folder_if_needed).
		// Folder could have been recreated between the early check and now.
		targetPath := filepath.Join(destDir, folderName)
		if _, err := os.Stat(targetPath); err == nil {
			if !promptOverwrite(targetPath) {
				os.Exit(1)
			}
		}
		if err := untarToDir(finalData, destDir); err != nil {
			fmt.Fprintf(os.Stderr, "ERR: untar: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "- folder save to "+folderName)
	} else {
		saveName := filename
		if outFile != "" {
			saveName = outFile
		}
		if err := os.WriteFile(saveName, finalData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "ERR: write %s: %v\n", saveName, err)
			os.Exit(1)
		}
		echoSimplifiedFC(saveName, finalData)
	}

	recordVisitor(rdb, slotInt, len(finalData), cost, "fr", ip, filename)
}

// echoSimplifiedFC prints file content (max 14 lines, truncated) then save-message to stderr.
// Matches Perl echo_simplified_fc exactly:
//   - Text detection via isTextData (Perl -T operator, ~10% threshold)
//   - Truncation: first 8 lines (0..$half inclusive) + " ..." + last 7 lines
//   - Output: say "\n", @fc  → blank line + content lines + trailing blank line (STDOUT)
//   - "- file content save to X" → STDERR
func echoSimplifiedFC(filename string, data []byte) {
	if isTextData(data) {
		lines := strings.Split(string(data), "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		const maxLines = 14
		if len(lines) > maxLines {
			half := maxLines / 2 // 7
			truncated := make([]string, 0, maxLines+2)
			truncated = append(truncated, lines[:half+1]...) // first 8 lines (0..7, inclusive — matches Perl @fc[0..$half])
			truncated = append(truncated, " ...")
			truncated = append(truncated, lines[len(lines)-half:]...) // last 7
			lines = truncated
		}
		// Perl: say "\n", @fc  — blank line before content, extra blank line after (say adds \n)
		fmt.Print("\n")
		fmt.Println(strings.Join(lines, "\n"))
		fmt.Println() // trailing blank line — matches Perl say's implicit \n after @fc
	}
	fmt.Fprintln(os.Stderr, "- file content save to "+filename)
}

// promptOverwrite checks if path exists and asks user to confirm overwrite.
// Matches Perl gen_overwrite_struct + real_run_prompt_and_input_yes.
func promptOverwrite(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	typeStr := "file"
	if info.IsDir() {
		typeStr = "folder"
	}
	// Perl: say $hash_->{overwrite_stat} — uses plain `say` which defaults to STDOUT
	fmt.Fprintf(os.Stdout, "- exists %s %s, would you like to overwrite? (yes | no)\n", typeStr, path)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(scanner.Text())
		if answer == "yes" || answer == "y" || answer == "Y" || answer == "YES" {
			if info.IsDir() {
				os.RemoveAll(path)
			} else {
				os.Remove(path)
			}
			return true
		}
	}
	fmt.Fprintf(os.Stderr, "- exist %s !\n", path)
	return false
}

func parseSlotInt(slotNum string) int {
	var n int
	fmt.Sscanf(slotNum, "%d", &n)
	return n
}
