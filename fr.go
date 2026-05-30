package main

import (
	"fmt"
	"os"
	"strings"
)

// execFr implements the `fr`/`f` command.
// - No slot arg: auto-uses current jd_incr (latest sent slot)
// - Accepts both "51" and "jd_51"
// - Prints content to stdout, saves to file, prints metadata to stderr
func execFr(cfg Config, args []string, password string, outFile string) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()

	checkVersion(rdb)

	var slotNum string

	if len(args) > 0 {
		// Strip jd_ prefix if present
		slotNum = strings.TrimPrefix(args[0], JD_SLOT_PREFIX)
	} else {
		// No arg: use current jd_incr to find last sent slot
		val, err := rdb.Get(ctx, JD_INCR_KEY).Int64()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR: get jd_incr: %v\n", err)
			os.Exit(1)
		}
		slotNum = fmt.Sprintf("%d", (val-1+int64(cfg.MaxJdIncr))%int64(cfg.MaxJdIncr))
	}

	slotKey := fmt.Sprintf("%s%s", JD_SLOT_PREFIX, slotNum)

	// Password check
	pwKey := fmt.Sprintf("%s%s", PW_PREFIX, slotNum)
	storedPw, err := rdb.Get(ctx, pwKey).Result()
	if err == nil && storedPw != "" {
		if password == "" {
			fmt.Fprint(os.Stderr, "请输入邀请码：")
			fmt.Scan(&password)
		}
		inputCrc := fmt.Sprintf("%d", mycrc32([]byte(password)))
		if inputCrc != strings.TrimSpace(storedPw) {
			fmt.Fprintln(os.Stderr, "ERR: wrong password")
			os.Exit(1)
		}
	}

	raw := redisGetWithProgress(rdb, slotKey, "")

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

	isFolder := strings.HasPrefix(filename, "FOLDER_")

	if isFolder {
		// Folder: extract, no stdout content
		destDir := "."
		if outFile != "" {
			destDir = outFile
		}
		if err := untarBytes(finalData, destDir); err != nil {
			fmt.Fprintf(os.Stderr, "ERR: untar: %v\n", err)
			os.Exit(1)
		}
		folderName := strings.TrimPrefix(filename, "FOLDER_")
		fmt.Fprintf(os.Stderr, "\n- folder extracted to %s/%s\n", destDir, folderName)
	} else {
		// Regular file: print content to stdout, save to file
		os.Stdout.Write(finalData)

		saveName := filename
		if outFile != "" {
			saveName = outFile
		}
		if err := os.WriteFile(saveName, finalData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "ERR: write %s: %v\n", saveName, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "\n- file content save to %s\n", saveName)
	}
}

