package main

import (
	"fmt"
	"os"
	"strings"
)

// execFr implements the `fr` command:
//   fr [slot]  [-pw password]  [-o outfile]
// Reads from Redis slot, verifies CRC32, decompresses, writes file.
func execFr(cfg Config, args []string, password string, outFile string) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()

	checkVersion(rdb)

	// Determine slot: if arg given use it, otherwise read from stdin prompt
	var slotKey string
	if len(args) > 0 {
		slotKey = fmt.Sprintf("%s%s", JD_SLOT_PREFIX, args[0])
	} else {
		// Interactive: ask for slot
		fmt.Print("Enter slot number: ")
		var slot int
		fmt.Scan(&slot)
		slotKey = fmt.Sprintf("%s%d", JD_SLOT_PREFIX, slot)
	}

	// Extract slot number for password/filename key lookup
	slotNum := strings.TrimPrefix(slotKey, JD_SLOT_PREFIX)

	// Password check
	pwKey := fmt.Sprintf("%s%s", PW_PREFIX, slotNum)
	storedPw, err := rdb.Get(ctx, pwKey).Result()
	if err == nil && storedPw != "" {
		// Password required
		if password == "" {
			fmt.Print("请输入邀请码：")
			fmt.Scan(&password)
		}
		inputCrc := fmt.Sprintf("%d", mycrc32([]byte(password)))
		if inputCrc != strings.TrimSpace(storedPw) {
			fmt.Println("ERR: wrong password")
			os.Exit(1)
		}
	}

	// Fetch data
	raw := redisGetWithProgress(rdb, slotKey, fmt.Sprintf("Downloading slot %s", slotNum))

	// Parse header
	isGzip, filename, storedCrc, payload, err := parseMetaHeader(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: parse header: %v\n", err)
		os.Exit(1)
	}

	// Decompress if needed
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

	// CRC32 check
	computedCrc := mycrc32(finalData)
	if computedCrc != storedCrc {
		fmt.Fprintf(os.Stderr, "ERR: CRC32 mismatch: got %d, want %d\n", computedCrc, storedCrc)
		os.Exit(1)
	}

	// Write output
	isFolder := strings.HasPrefix(filename, "FOLDER_")
	if isFolder {
		destDir := "."
		if outFile != "" {
			destDir = outFile
		}
		if err := untarBytes(finalData, destDir); err != nil {
			fmt.Fprintf(os.Stderr, "ERR: untar: %v\n", err)
			os.Exit(1)
		}
		folderName := strings.TrimPrefix(filename, "FOLDER_")
		fmt.Printf("OK: extracted folder %q to %s  size=%d  crc32=%d\n",
			folderName, destDir, len(finalData), storedCrc)
	} else {
		out := filename
		if outFile != "" {
			out = outFile
		}
		if err := os.WriteFile(out, finalData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "ERR: write %s: %v\n", out, err)
			os.Exit(1)
		}
		fmt.Printf("OK: wrote %s  size=%d  crc32=%d\n", out, len(finalData), storedCrc)
	}
}
