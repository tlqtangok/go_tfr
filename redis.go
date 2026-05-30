package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TFR_VERSION     = "2019.04.01"
	VERSION_KEY     = "TOR_FR_VERSION_KEY"
	JD_INCR_KEY     = "jd_incr"
	JD_SLOT_PREFIX  = "jd_"
	FNAME_PREFIX    = "FILENAME_:jd_"
	PW_PREFIX       = "PW_OF_:jd_"
	EXPIRY          = 3 * time.Hour
)

var ctx = context.Background()

func newRedisClient(cfg Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DialTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})
}

func checkVersion(rdb *redis.Client) {
	v, err := rdb.Get(ctx, VERSION_KEY).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: cannot connect to Redis or version key missing: %v\n", err)
		os.Exit(1)
	}
	if v != TFR_VERSION {
		fmt.Fprintf(os.Stderr, "ERR: version mismatch: got %q, want %q\n", v, TFR_VERSION)
		os.Exit(1)
	}
}

// getSlot atomically increments jd_incr and returns the slot index (0-based, mod maxJdIncr).
func getSlot(rdb *redis.Client, maxJdIncr int) int {
	val, err := rdb.Incr(ctx, JD_INCR_KEY).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: incr jd_incr: %v\n", err)
		os.Exit(1)
	}
	return int((val - 1) % int64(maxJdIncr))
}

// redisSetWithProgress stores data in Redis; progress bar only for large payloads.
func redisSetWithProgress(rdb *redis.Client, key string, data []byte, label string) {
	err := rdb.Set(ctx, key, data, EXPIRY).Err()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nERR: redis SET %s: %v\n", key, err)
		os.Exit(1)
	}
}

// redisGetWithProgress retrieves data from Redis; progress bar only for large payloads.
func redisGetWithProgress(rdb *redis.Client, key string, label string) []byte {
	data, err := rdb.Get(ctx, key).Bytes()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: redis GET %s: %v\n", key, err)
		os.Exit(1)
	}
	return data
}
