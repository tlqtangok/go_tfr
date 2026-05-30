package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

func main() {
	cfg := loadConfig()

	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "-v":
		fmt.Print("- tor-fr\n" +
			"  version: 2019.04.01\n\n" +
			"  author: Jidor Tang<tlqtangok@126.com>\n" +
			"  homepage: http://jesson.tech:10241/tor_fr_readme.html\n\n" +
			"  usage:\n" +
			"    tfr tor <filename> -pw <your_password>\n" +
			"    tfr fr jd_xx\n\n" +
			"  description:\n" +
			"    tor-fr is a productive tool that sync-up and share your \n" +
			"    files,directories instantly, efficently and elegantly\n")
		os.Exit(0)
	case "tor", "t":
		fn, pw := parseFnPw(rest)
		execTor(cfg, fn, pw)
	case "fr", "f":
		fn, pw := parseFnPw(rest)
		execFr(cfg, fn, pw, "")
	case "sv":
		count := 0
		if len(rest) > 0 {
			count, _ = strconv.Atoi(rest[0])
		}
		showVisitor(cfg, count)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// parseFnPw parses [fn] [-pw password] or [-pw password] [fn] args.
func parseFnPw(args []string) ([]string, string) {
	pw := ""
	var fnArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-pw" && i+1 < len(args) {
			pw = args[i+1]
			i++
		} else {
			fnArgs = append(fnArgs, args[i])
		}
	}
	return fnArgs, pw
}

// readPassword reads a password without echo (matches Perl ReadMode noecho).
func readPassword(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(pw))
	}
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func printUsage() {
	fmt.Print("TFR - Transfer via Redis (Go rewrite)\n" +
		"Usage:\n" +
		"  tfr tor [file|folder] [-pw password]   send (t = alias)\n" +
		"  tfr fr  [jd_N|N]      [-pw password]   receive (f = alias)\n" +
		"  tfr sv  [count]                         show visitor log\n")
}
