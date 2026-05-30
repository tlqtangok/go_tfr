package main

import (
	"fmt"
	"os"
)

const VERSION = "go-tfr v1.0 (compatible with TFR 2019.04.01)"

func usage() {
	fmt.Printf(`%s

Usage:
  tfr tor <file|-|folder>  [-pw <password>]        Send file/stdin/folder
  tfr fr  [slot]           [-pw <password>] [-o <outfile>]  Receive
  tfr show_visitor         [-pw <password>]         Show transfer history
  tfr -v                                            Show version

Config: tfr.config next to binary (or ./tfr.config)
  $redis_host = 127.0.0.1;
  $redis_port = 10240;
  $max_file_sz_in_bytes = 52429824;
  $max_jd_incr = 256;
`, VERSION)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(0)
	}

	cfg := loadConfig()

	// Parse global flags
	args := os.Args[1:]
	var password string
	var outFile string
	var filtered []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-pw":
			if i+1 < len(args) {
				i++
				password = args[i]
			}
		case "-o":
			if i+1 < len(args) {
				i++
				outFile = args[i]
			}
		default:
			filtered = append(filtered, args[i])
		}
	}
	args = filtered

	if len(args) == 0 {
		usage()
		os.Exit(0)
	}

	switch args[0] {
	case "tor", "t":
		execTor(cfg, args[1:], password)
	case "fr", "f":
		execFr(cfg, args[1:], password, outFile)
	case "show_visitor":
		showVisitor(cfg, password)
	case "-v", "--version", "version":
		fmt.Println(VERSION)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		usage()
		os.Exit(1)
	}
}
