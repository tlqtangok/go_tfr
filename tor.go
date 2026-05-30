package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// execTor implements the `tor` command:
//   tor [file|-]  [-pw password]
// Reads file (or stdin if "-"), optionally gzips, builds metadata header, stores in Redis.
func execTor(cfg Config, args []string, password string) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()

	checkVersion(rdb)

	var filename string
	var rawData []byte
	var isFolder bool

	if len(args) == 0 || args[0] == "-" {
		filename = "stdin"
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
			fmt.Fprintf(os.Stderr, "ERR: stat %s: %v\n", target, err)
			os.Exit(1)
		}
		if info.IsDir() {
			isFolder = true
			filename = "FOLDER_" + info.Name()
			rawData, err = tarFolder(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERR: tar folder: %v\n", err)
				os.Exit(1)
			}
		} else {
			filename = info.Name()
			rawData, err = os.ReadFile(target)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERR: read file: %v\n", err)
				os.Exit(1)
			}
		}
	}

	_ = isFolder

	// Check size
	if int64(len(rawData)) > cfg.MaxFileSz {
		fmt.Fprintf(os.Stderr, "ERR: file too large (%d bytes, max %d)\n", len(rawData), cfg.MaxFileSz)
		os.Exit(1)
	}

	// Gzip if needed
	payload := rawData
	isGzip := false
	if len(rawData) > GZIP_THRESHOLD {
		compressed, err := gzipBytes(rawData)
		if err == nil && len(compressed) < len(rawData) {
			payload = compressed
			isGzip = true
		}
	}

	crc := mycrc32(rawData) // CRC of original data

	// Get slot
	slot := getSlot(rdb, cfg.MaxJdIncr)
	slotKey := fmt.Sprintf("%s%d", JD_SLOT_PREFIX, slot)
	fnKey := fmt.Sprintf("%s%d", FNAME_PREFIX, slot)

	// Build full Redis value: header + payload
	header := buildMetaHeader(isGzip, filename, crc)
	value := append(header, payload...)

	// Store password CRC if provided
	if password != "" {
		pwKey := fmt.Sprintf("%s%d", PW_PREFIX, slot)
		pwCrc := mycrc32([]byte(password))
		rdb.Set(ctx, pwKey, fmt.Sprintf("%d", pwCrc), EXPIRY)
	}

	// Store filename key
	rdb.Set(ctx, fnKey, filename, EXPIRY)

	// Record visitor info
	recordVisitor(rdb, slot, len(rawData), "tor")

	start := time.Now()
	redisSetWithProgress(rdb, slotKey, value, fmt.Sprintf("Uploading %s", filename))
	elapsed := time.Since(start)

	fmt.Printf("OK: slot=%d  file=%s  size=%d  gz=%v  crc32=%d  time=%.2fs\n",
		slot, filename, len(rawData), isGzip, crc, elapsed.Seconds())
}

// recordVisitor appends a visitor log entry to Redis list "jd_visitor".
func recordVisitor(rdb *redis.Client, slot int, size int, op string) {
	entry := fmt.Sprintf("%s slot=%d size=%d op=%s", time.Now().Format("2006-01-02 15:04:05"), slot, size, op)
	rdb.LPush(ctx, "jd_visitor", entry)
	rdb.LTrim(ctx, "jd_visitor", 0, 999) // keep last 1000
}

// showVisitor implements the `show_visitor` subcommand (password protected on JDLS side).
func showVisitor(cfg Config, password string) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()

	checkVersion(rdb)

	if password != "" {
		// Check against stored visitor password (key: "VISITOR_PW")
		stored, err := rdb.Get(ctx, "VISITOR_PW").Result()
		if err == nil {
			inputCrc := fmt.Sprintf("%d", mycrc32([]byte(password)))
			if inputCrc != strings.TrimSpace(stored) {
				fmt.Println("ERR: wrong password")
				os.Exit(1)
			}
		}
	}

	entries, err := rdb.LRange(ctx, "jd_visitor", 0, 49).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: %v\n", err)
		os.Exit(1)
	}
	for _, e := range entries {
		fmt.Println(e)
	}
}
