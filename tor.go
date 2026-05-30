package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
)

// execTor implements the tor/t command.
func execTor(cfg Config, args []string, password string) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()

	checkVersion(rdb)

	var filename string
	var rawData []byte
	var isFolder bool

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

	if int64(len(rawData)) > cfg.MaxFileSz {
		fmt.Fprintf(os.Stderr, "ERR: file too large (%d bytes, max %d)\n", len(rawData), cfg.MaxFileSz)
		os.Exit(1)
	}

	payload := rawData
	isGzip := false
	if len(rawData) > GZIP_THRESHOLD {
		compressed, err := gzipBytes(rawData)
		if err == nil && len(compressed) < len(rawData) {
			payload = compressed
			isGzip = true
		}
	}

	crc := mycrc32(rawData)
	slot := getSlot(rdb, cfg.MaxJdIncr)
	slotKey := fmt.Sprintf("%s%d", JD_SLOT_PREFIX, slot)
	fnKey := fmt.Sprintf("%s%d", FNAME_PREFIX, slot)

	header := buildMetaHeader(isGzip, filename, crc)
	value := append(header, payload...)

	if password != "" {
		pwKey := fmt.Sprintf("%s%d", PW_PREFIX, slot)
		pwCrc := mycrc32([]byte(password))
		rdb.Set(ctx, pwKey, fmt.Sprintf("%d", pwCrc), EXPIRY)
	}

	rdb.Set(ctx, fnKey, filename, EXPIRY)
	recordVisitor(rdb, slot, len(rawData), "tor")
	redisSetWithProgress(rdb, slotKey, value, "")

	fmt.Printf("jd_%d\n", slot)
}

func recordVisitor(rdb *redis.Client, slot int, size int, op string) {
	entry := fmt.Sprintf("slot=%d size=%d op=%s", slot, size, op)
	rdb.LPush(ctx, "jd_visitor", entry)
	rdb.LTrim(ctx, "jd_visitor", 0, 999)
}

func showVisitor(cfg Config, password string) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()

	checkVersion(rdb)

	if password != "" {
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
