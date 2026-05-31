package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	TFR_VERSION    = "2019.04.01"
	VERSION_KEY    = "TOR_FR_VERSION_KEY"
	JD_INCR_KEY    = "jd_incr"
	JD_SLOT_PREFIX = "jd_"
	FNAME_PREFIX   = "FILENAME_:jd_"
	PW_PREFIX      = "PW_OF_:jd_"
	VISITOR_KEY    = "VISITOR_"
	EXPIRY         = 3 * time.Hour
)

var ctx = context.Background()

func newRedisClient(cfg Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DialTimeout:  5 * time.Second,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		PoolSize:     5,
	})
}

// countingConn wraps net.Conn to count bytes flowing through it for progress tracking.
type countingConn struct {
	net.Conn
	bytesRead    *int64 // may be nil
	bytesWritten *int64 // may be nil
}

func (c *countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if c.bytesRead != nil {
		atomic.AddInt64(c.bytesRead, int64(n))
	}
	return n, err
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if c.bytesWritten != nil {
		atomic.AddInt64(c.bytesWritten, int64(n))
	}
	return n, err
}

// newLargeOpClient creates a single-connection Redis client that counts bytes for progress.
// Pass non-nil pointers to track reads (download) or writes (upload).
func newLargeOpClient(cfg Config, bytesRead, bytesWritten *int64) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DialTimeout:  30 * time.Second,
		ReadTimeout:  largeOpTimeout,
		WriteTimeout: largeOpTimeout,
		PoolSize:     1,
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			return &countingConn{Conn: conn, bytesRead: bytesRead, bytesWritten: bytesWritten}, nil
		},
	})
}

// checkVersion verifies server TFR version. Supports "version:die" format.
func checkVersion(rdb *redis.Client) {
	v, err := rdb.Get(ctx, VERSION_KEY).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: cannot connect to Redis or version key missing: %v\n", err)
		os.Exit(1)
	}
	mustDie := false
	if strings.Contains(v, ":") {
		parts := strings.SplitN(v, ":", 2)
		v = parts[0]
		if parts[1] == "die" {
			mustDie = true
		}
	}
	if v != TFR_VERSION {
		if mustDie {
			fmt.Fprintf(os.Stderr, "ERR: version mismatch (die): server=%q, client=%q\n", v, TFR_VERSION)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "WARN: version mismatch: server=%q, client=%q\n", v, TFR_VERSION)
	}
}

// getSlot increments jd_incr and returns slot number.
// Matches Perl incr_jd_incr: INCR; if >= max then SET to 0 and return 0.
// Slots cycle: 1, 2, ..., max-1, 0, 1, 2, ...
func getSlot(rdb *redis.Client, maxJdIncr int) int {
	val, err := rdb.Incr(ctx, JD_INCR_KEY).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: incr jd_incr: %v\n", err)
		os.Exit(1)
	}
	if val >= int64(maxJdIncr) {
		rdb.Set(ctx, JD_INCR_KEY, 0, 0)
		return 0
	}
	return int(val)
}

// getCurrentSlotNum reads current jd_incr without incrementing (for fr without args).
// Matches Perl get_jd_xx_from_incr: GET jd_incr; use value directly.
func getCurrentSlotNum(rdb *redis.Client) string {
	val, err := rdb.Get(ctx, JD_INCR_KEY).Result()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERR: get jd_incr: %v\n", err)
		os.Exit(1)
	}
	return val
}

// clearAllJdKeys deletes all jd_* / PW_OF_:jd_* / FILENAME_:jd_* keys.
// Called when slot wraps to 0 (matches Perl clear_all_jd_xx_and_pw_prefix).
func clearAllJdKeys(rdb *redis.Client) {
	patterns := []string{"jd_*", "PW_OF_:jd_*", "FILENAME_:jd_*"}
	for _, pat := range patterns {
		var cursor uint64
		for {
			keys, next, err := rdb.Scan(ctx, cursor, pat, 100).Result()
			if err != nil {
				break
			}
			if len(keys) > 0 {
				rdb.Del(ctx, keys...)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
}

// getClientIP retrieves current client IP via Redis CLIENT LIST (matches Perl).
func getClientIP(rdb *redis.Client) string {
	time.Sleep(200 * time.Millisecond)
	result, err := rdb.Do(ctx, "CLIENT", "LIST").Result()
	if err != nil {
		return "unknown"
	}
	listStr, ok := result.(string)
	if !ok {
		return "unknown"
	}
	lines := strings.Split(strings.TrimSpace(listStr), "\n")
	if len(lines) == 0 {
		return "unknown"
	}
	lastLine := lines[len(lines)-1]
	for _, field := range strings.Fields(lastLine) {
		if strings.HasPrefix(field, "addr=") {
			addrPort := strings.TrimPrefix(field, "addr=")
			if idx := strings.LastIndex(addrPort, ":"); idx > 0 {
				return addrPort[:idx]
			}
		}
	}
	return "unknown"
}

// recordVisitor appends visitor log (matches Perl record_ip_ts_len_cost).
// Format: YYYYMMDD_HHMM\top\tip\tlen\tcost\tjd_xx\tfn  Key: VISITOR_  cmd: rpush
func recordVisitor(rdb *redis.Client, slot int, dataLen int, costSecs float64, op, ip, fn string) {
	ts := time.Now().Format("20060102_1504")
	jdXX := fmt.Sprintf("jd_%d", slot)
	entry := strings.Join([]string{ts, op, ip, fmt.Sprintf("%d", dataLen),
		fmt.Sprintf("%.2f", costSecs), jdXX, fn}, "\t")
	rdb.RPush(ctx, VISITOR_KEY, entry)
}

const (
	largeOpTimeout = 30 * time.Minute // large data GET/SET can be slow
	maxRetries     = 3
	retryDelay     = 4 * time.Second
)

func redisSet(rdb *redis.Client, key string, data []byte) {
	cli := rdb.WithTimeout(largeOpTimeout)
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = cli.Set(ctx, key, data, EXPIRY).Err()
		if err == nil {
			return
		}
		if attempt < maxRetries {
			fmt.Fprintf(os.Stderr, "- redis SET transient error, retrying in 4s... (%d/%d): %v\n", attempt, maxRetries, err)
			time.Sleep(retryDelay)
		}
	}
	fmt.Fprintf(os.Stderr, "ERR: redis SET %s: %v\n", key, err)
	os.Exit(1)
}

func redisGet(rdb *redis.Client, key string) []byte {
	cli := rdb.WithTimeout(largeOpTimeout)
	var data []byte
	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		data, err = cli.Get(ctx, key).Bytes()
		if err == nil {
			return data
		}
		if attempt < maxRetries {
			fmt.Fprintf(os.Stderr, "- redis GET transient error, retrying in 4s... (%d/%d): %v\n", attempt, maxRetries, err)
			time.Sleep(retryDelay)
		}
	}
	fmt.Fprintf(os.Stderr, "ERR: redis GET %s: %v\n", key, err)
	os.Exit(1)
	return nil
}

func showVisitor(cfg Config, count int) {
	rdb := newRedisClient(cfg)
	defer rdb.Close()
	checkVersion(rdb)

	ip := getClientIP(rdb)
	needPW := !(strings.HasPrefix(ip, "116.") || ip == "127.0.0.1")
	if needPW {
		ckAdminPassword()
	}

	var entries []string
	var rErr error
	if count > 0 {
		entries, rErr = rdb.LRange(ctx, VISITOR_KEY, int64(-count), -1).Result()
	} else {
		entries, rErr = rdb.LRange(ctx, VISITOR_KEY, 0, -1).Result()
	}
	if rErr != nil {
		fmt.Fprintf(os.Stderr, "ERR: %v\n", rErr)
		os.Exit(1)
	}
	for _, e := range entries {
		fmt.Println(e)
	}
}

// ckAdminPassword checks the time-based admin password for show_visitor.
// Matches Perl: mycrc32(input) == mycrc32("JD_DISABLE_PW_" + YYYYMMDD)
// The correct password for today is: JD_DISABLE_PW_YYYYMMDD
func ckAdminPassword() {
	today := time.Now().Format("20060102") // YYYYMMDD
	expected := mycrc32([]byte("JD_DISABLE_PW_" + today))

	pw := readPassword("- need admin password, please input: ")
	if mycrc32([]byte(pw)) != expected {
		fmt.Fprint(os.Stderr, "  incorrect password, try again\n")
		os.Exit(1)
	}
}
