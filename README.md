# TFR — Transfer via Redis

[![Release](https://img.shields.io/github/v/release/tlqtangok/go_tfr)](https://github.com/tlqtangok/go_tfr/releases)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

> **tor-fr** is a productive tool that syncs and shares your files, directories instantly, efficiently and elegantly — via Redis as the transport layer.

Go rewrite of the original [Perl TFR](https://github.com/tlqtangok/tfr).  
Wire-compatible with the Perl version: Go ↔ Perl cross-transfer works seamlessly.

---

## Features

- **Zero setup** on the receiver side — just run `tfr f`
- Transfer files, directories, or piped stdin
- Password protection per transfer
- Folder auto-tar + compress
- 256 rotating slots (`jd_0` ~ `jd_255`)
- Visitor log (`sv` command)
- Cross-platform: Linux / Windows / macOS

---

## Install

Download the binary for your platform from [Releases](https://github.com/tlqtangok/go_tfr/releases) and put it in your `$PATH`:

```bash
# Linux x86_64
curl -L https://github.com/tlqtangok/go_tfr/releases/latest/download/tfr_linux_amd64 -o /usr/local/bin/tfr
chmod +x /usr/local/bin/tfr
```

---

## Configuration

Place `tfr.config` in the same directory as the binary (or current working directory):

```ini
$redis_host = your.redis.host;
$redis_port = 6379;
$max_file_sz_in_bytes = 52429824;   # 50 MB
$max_jd_incr = 256;
```

---

## Usage

### `tor` — Send (Transfer Out via Redis)

```bash
# Send a file
tfr tor <filename>
tfr t   <filename>          # shorthand

# Send with password
tfr t <filename> -pw <password>

# Send a directory (auto tar+gzip)
tfr t <dirname>

# Pipe stdin
echo "hello world" | tfr t
cat file.txt       | tfr t
```

Output: the slot ID, e.g. `jd_42`

---

### `fr` — Receive (Fetch via Redis)

```bash
# Receive latest slot (no args)
tfr fr
tfr f               # shorthand

# Receive a specific slot
tfr f jd_42

# Receive with password
tfr f jd_42 -pw <password>

# Pipe output directly to shell
tfr f | bash
```

- Text files: content is printed to stdout, saved to `txt.txt`
- Binary/archive files: saved to original filename in current directory

---

### `sv` — Show Visitors

```bash
# Show last 10 visitors
tfr sv

# Show last N visitors
tfr sv 30
```

---

### Version / Help

```bash
tfr version
```

---

## Examples

```bash
# ── Machine A (sender) ────────────────────────
$ echo "deploy this" | tfr t
jd_17

$ tfr t ~/projects/myapp -pw secret
jd_18

# ── Machine B (receiver) ──────────────────────
$ tfr f
deploy this
- file content save to txt.txt

$ tfr f jd_18 -pw secret
- extracting myapp/...

# ── Pipe trick ────────────────────────────────
$ echo "ls -la" | tfr t
jd_19

$ tfr f jd_19 | bash
total 48
drwxr-xr-x ...
```

---

## Slots

TFR uses a rotating counter `jd_incr` (0–255) stored in Redis.  
Each `tfr t` call increments the counter and assigns a slot (`jd_1`, `jd_2`, … `jd_255`, then wraps to `jd_0`).  
All slot data expires after **3 hours**.  
When the counter wraps to 0, all existing `jd_*` keys are cleared automatically.

---

## Build from Source

**Requirements:** Go 1.22+, optionally UPX for compressed binaries.

```bash
git clone https://github.com/tlqtangok/go_tfr.git
cd go_tfr

# Build current platform
./build.sh linux

# Build all platforms
./build.sh all

# Clean
./build.sh clean
```

Or manually:

```bash
go build -ldflags="-s -w" -trimpath -o tfr .
```

---

## Compatibility

| Sender | Receiver | Status |
|--------|----------|--------|
| Go TFR | Go TFR   | ✅ |
| Go TFR | Perl TFR | ✅ |
| Perl TFR | Go TFR | ✅ |
| Perl TFR | Perl TFR | ✅ |

---

## Author

**Jidor Tang** <tlqtangok@126.com>  
GitHub: [@tlqtangok](https://github.com/tlqtangok)

Original Perl version co-authored with Jesson Liu ([@LjessonS](https://github.com/LjessonS))

---

## License

Apache License 2.0 — see [LICENSE](LICENSE)
