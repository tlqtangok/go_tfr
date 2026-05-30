# go_tfr

Go rewrite of [TFR](https://github.com/tlqtangok/tfr) — a Redis-based file transfer tool. 100% compatible with the original Perl version.

## Features

- Send/receive files, stdin, and folders via Redis
- Gzip compression for files > 2KB
- CRC32 integrity verification
- Password protection per transfer
- Progress bar display
- Static binary — no runtime dependencies

## Usage

```bash
# Send a file
tfr tor myfile.txt

# Send a folder
tfr tor myfolder/

# Send with password
tfr tor myfile.txt -pw secret

# Receive (auto-detects filename from slot)
tfr fr <slot>

# Receive with password
tfr fr <slot> -pw secret

# Receive and save to specific path
tfr fr <slot> -o /path/to/output

# Show transfer history
tfr show_visitor

# Version
tfr -v
```

## Config (tfr.config, same dir as binary)

```perl
$redis_host = 127.0.0.1;
$redis_port = 10240;
$max_file_sz_in_bytes = 52429824;
$max_jd_incr = 256;
```

## Compatibility

Fully wire-compatible with the original Perl TFR:

| Sender | Receiver | Status |
|--------|----------|--------|
| Go tfr | Go tfr   | ✅ |
| Go tfr | Perl tfr | ✅ |
| Perl tfr | Go tfr | ✅ |

## Build

```bash
make linux    # Linux amd64
make windows  # Windows amd64
make arm      # Linux arm64 (Raspberry Pi)
make darwin   # macOS amd64
```

## Download

See [Releases](https://github.com/tlqtangok/go_tfr/releases) for pre-built binaries.
rewrite tfr project with Go.
