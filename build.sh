#!/usr/bin/env bash
# export PATH=$PATH:/usr/local/go/bin
# build.sh — build all TFR binaries for all platforms
#
# Usage: ./build.sh
#
# Output:
#   tfr_linux_amd64        Linux x86_64       (UPX --brute, ~1.8 MB)
#   tfr_linux_arm64        Linux ARM64        (UPX --brute, ~1.6 MB)
#   tfr_windows_amd64.exe  Windows x86_64     (UPX --brute, ~1.9 MB)
#   tfr_darwin_amd64       macOS Intel        (no UPX,      ~5 MB — macOS rejects packed binaries)
#   tfr_darwin_arm64       macOS Apple Silicon (no UPX,      ~5 MB — macOS rejects packed binaries)

set -euo pipefail

# ── helpers ───────────────────────────────────────────────────────────────────

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info()  { echo -e "${GREEN}[+]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
die()   { echo -e "${RED}[x]${NC} $*" >&2; exit 1; }

require() { command -v "$1" &>/dev/null || die "'$1' not found — please install it first"; }

# UPX refuses to write to a file that already exists when using -o.
# Always remove the destination before calling upx_compress.
upx_compress() {
    local level=$1 src=$2 dst=$3
    rm -f "$dst"
    upx "$level" -o "$dst" "$src" &>/dev/null
    info "  $(ls -lh "$dst" | awk '{print $5, $9}')"
}

# ── prerequisites ─────────────────────────────────────────────────────────────

require go
require upx

export GOTOOLCHAIN=local
export CGO_ENABLED=0
LDFLAGS="-s -w"

info "Go:  $(go version)"
info "UPX: $(upx --version 2>&1 | head -1)"
echo

# ── step 1: compile raw binaries ─────────────────────────────────────────────

info "[1/2] Compiling for all platforms..."

GOOS=linux   GOARCH=amd64 go build -ldflags="$LDFLAGS" -trimpath -o _raw_linux_amd64       . && info "  linux/amd64   ok"
GOOS=linux   GOARCH=arm64 go build -ldflags="$LDFLAGS" -trimpath -o _raw_linux_arm64       . && info "  linux/arm64   ok"
GOOS=windows GOARCH=amd64 go build -ldflags="$LDFLAGS" -trimpath -o _raw_windows_amd64.exe . && info "  windows/amd64 ok"
GOOS=darwin  GOARCH=amd64 go build -ldflags="$LDFLAGS" -trimpath -o tfr_darwin_amd64       . && info "  darwin/amd64  ok (final — no UPX)"
GOOS=darwin  GOARCH=arm64 go build -ldflags="$LDFLAGS" -trimpath -o tfr_darwin_arm64       . && info "  darwin/arm64  ok (final — no UPX)"

# ── step 2: UPX --brute for Linux/Windows main binaries ──────────────────────

echo
info "[2/2] Compressing with UPX --brute (smallest size — Linux + Windows)..."

upx_compress --brute _raw_linux_amd64       tfr_linux_amd64
upx_compress --brute _raw_linux_arm64       tfr_linux_arm64
upx_compress --brute _raw_windows_amd64.exe tfr_windows_amd64.exe

# ── cleanup raw intermediates ─────────────────────────────────────────────────

rm -f _raw_linux_amd64 _raw_linux_arm64 _raw_windows_amd64.exe

# ── summary ───────────────────────────────────────────────────────────────────

echo
info "All done! Final binaries:"
ls -lh tfr_linux_amd64 tfr_linux_arm64 tfr_windows_amd64.exe tfr_darwin_amd64 tfr_darwin_arm64
