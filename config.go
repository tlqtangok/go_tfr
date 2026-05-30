package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Config struct {
	Host         string
	Port         int
	MaxFileSz    int64
	MaxJdIncr    int
	Password     string // optional, from -pw flag
}

func defaultConfig() Config {
	return Config{
		Host:      "127.0.0.1",
		Port:      10240,
		MaxFileSz: 52429824, // 50MB + 5KB
		MaxJdIncr: 256,
	}
}

// configPath returns the path to tfr.config next to the binary.
func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "tfr.config"
	}
	return filepath.Join(filepath.Dir(exe), "tfr.config")
}

// loadConfig parses a Perl-style config:  $varname = value;
func loadConfig() Config {
	cfg := defaultConfig()
	path := configPath()
	f, err := os.Open(path)
	if err != nil {
		// Also try current dir
		f, err = os.Open("tfr.config")
		if err != nil {
			return cfg
		}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Remove leading $
		line = strings.TrimPrefix(line, "$")
		// Remove trailing ;
		line = strings.TrimSuffix(line, ";")
		// Split on first =
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip quotes
		val = strings.Trim(val, `"'`)

		switch key {
		case "redis_host":
			cfg.Host = val
		case "redis_port":
			if p, err := strconv.Atoi(val); err == nil {
				cfg.Port = p
			}
		case "max_file_sz_in_bytes":
			if sz, err := strconv.ParseInt(val, 10, 64); err == nil {
				cfg.MaxFileSz = sz
			}
		case "max_jd_incr":
			if m, err := strconv.Atoi(val); err == nil {
				cfg.MaxJdIncr = m
			}
		}
	}
	return cfg
}

// isWindows returns true on Windows.
func isWindows() bool {
	return runtime.GOOS == "windows"
}
