package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// execTor implements the tor/t command.
// Key behaviors matching Perl exec_tor_process:
// - Print slot BEFORE processing file (Perl outputs jd_xx then reads file)
// - Skip gzip for .tar.gz files (already compressed)
// - Use pipeline for Redis writes (performance)
// - Clear all jd_* keys on slot wrap to 0
func execTor(cfg Config, args []string, password string) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()

	checkVersion(rdb)

	var filename string
	var rawData []byte

	if len(args) == 0 || args[0] == "-" {
		filename = "txt.txt"
		var err error
		rawData, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERR: read stdin: %v\n", err)
			os.Exit(1)
		}
	} else {
		target := args[0]
		info, err := os.Stat(target)
		if err != nil {
			// Non-existent arg: treat as inline string (like Perl)
			rawData = append([]byte(target), '\n')
			filename = "txt.txt"
		} else if info.IsDir() {
			// Folder: tar.gz it; filename = FOLDER_name.tar.gz (Perl convention)
			filename = fmt.Sprintf("FOLDER_%s.tar.gz", info.Name())
			rawData, err = tarFolder(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERR: tar folder: %v\n", err)
				os.Exit(1)
			}
		} else {
			filename = filepath.Base(target)
			rawData, err = os.ReadFile(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERR: read file: %v\n", err)
				os.Exit(1)
			}
		}
	}

	if int64(len(rawData)) > cfg.MaxFileSz {
		fmt.Fprintf(os.Stderr, "ERR: file too large (%d bytes, max %d)\n", len(rawData), cfg.MaxFileSz)
		os.Exit(1)
	}

	// Claim slot first — print jd_XX BEFORE processing (matches Perl exec_tor_process line order)
	slot := getSlot(rdb, cfg.MaxJdIncr)
	jdXX := fmt.Sprintf("jd_%d", slot)
	fmt.Println(jdXX)

	// Housekeeping: clear all old keys on slot wrap (Perl: clear_all_jd_xx_and_pw_prefix)
	if slot == 0 {
		clearAllJdKeys(rdb)
	}

	// Gzip: skip for .tar.gz files (matches Perl: $fn !~ /\.tar\.gz$/)
	payload := rawData
	isGzip := false
	if len(rawData) > GZIP_THRESHOLD && !strings.HasSuffix(filename, ".tar.gz") {
		compressed, err := gzipBytes(rawData)
		if err == nil && len(compressed) < len(rawData) {
			payload = compressed
			isGzip = true
		}
	}

	crc := mycrc32(rawData)
	slotKey := fmt.Sprintf("%s%d", JD_SLOT_PREFIX, slot)
	fnKey := fmt.Sprintf("%s%d", FNAME_PREFIX, slot)

	header := buildMetaHeader(isGzip, filename, crc)
	value := make([]byte, 0, len(header)+len(payload))
	value = append(value, header...)
	value = append(value, payload...)

	startTime := time.Now()

	// Show progress bar if transfer is expected to take > 10s (matches Perl)
	estSec := estimateSec(int64(len(value)), netSpeedUpKBs)
	largeCli := rdb.WithTimeout(largeOpTimeout)
	var pipeErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		pipe := largeCli.Pipeline()
		pipe.Set(ctx, slotKey, value, EXPIRY)
		pipe.Set(ctx, fnKey, filename, EXPIRY)
		if password != "" {
			pwKey := fmt.Sprintf("%s%d", PW_PREFIX, slot)
			pwCrc := mycrc32([]byte(password))
			pipe.Set(ctx, pwKey, fmt.Sprintf("%d", pwCrc), EXPIRY)
		}
		runWithProgress(estSec, func() {
			_, pipeErr = pipe.Exec(ctx)
		})
		if pipeErr == nil {
			break
		}
		if attempt < maxRetries {
			fmt.Fprintf(os.Stderr, "- upload transient error, retrying in 4s... (%d/%d): %v\n", attempt, maxRetries, pipeErr)
			time.Sleep(retryDelay)
		}
	}
	if pipeErr != nil {
		fmt.Fprintf(os.Stderr, "ERR: redis pipeline: %v\n", pipeErr)
		os.Exit(1)
	}

	cost := time.Since(startTime).Seconds()
	ip := getClientIP(rdb)
	recordVisitor(rdb, slot, len(rawData), cost, "tor", ip, filename)
}
