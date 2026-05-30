#!/usr/bin/env bash
set -e

# ─────────────────────────────────────────────
#  TFR Go build script
#  Usage: ./build.sh [all|linux|windows|darwin|clean]
# ─────────────────────────────────────────────

GO=${GO:-$(which go 2>/dev/null || echo "")}
UPX=$(which upx 2>/dev/null || echo "")

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
error() { echo -e "${RED}[x]${NC} $*"; exit 1; }

# ── env check ──────────────────────────────────
check_env() {
    info "Checking environment..."

    [[ -z "$GO" ]] && error "Go not found. Install from https://go.dev/dl/"

    GO_VER=$("$GO" version | grep -oP 'go\K[0-9]+\.[0-9]+')
    GO_MAJ=$(echo "$GO_VER" | cut -d. -f1)
    GO_MIN=$(echo "$GO_VER" | cut -d. -f2)
    if [[ "$GO_MAJ" -lt 1 ]] || ( [[ "$GO_MAJ" -eq 1 ]] && [[ "$GO_MIN" -lt 22 ]] ); then
        error "Go >= 1.22 required, found go${GO_VER}"
    fi
    info "Go: $("$GO" version)"

    if [[ -z "$UPX" ]]; then
        warn "UPX not found — binaries won't be compressed (apt install upx-ucl)"
    else
        info "UPX: $(upx --version 2>&1 | head -1)"
    fi
}

# ── deps ───────────────────────────────────────
fetch_deps() {
    info "Fetching dependencies..."
    GOTOOLCHAIN=local "$GO" mod tidy
}

# ── build one target ───────────────────────────
build_one() {
    local OS=$1 ARCH=$2 OUT=$3
    info "Building ${OS}/${ARCH} → ${OUT}"
    GOTOOLCHAIN=local CGO_ENABLED=0 GOOS="$OS" GOARCH="$ARCH" \
        "$GO" build -ldflags="-s -w" -trimpath -o "$OUT" .

    if [[ -n "$UPX" && "$OS" != "darwin" ]]; then
        upx --best "$OUT" > /dev/null 2>&1 && \
            info "  UPX compressed: $(du -sh "$OUT" | cut -f1)"
    else
        info "  Size: $(du -sh "$OUT" | cut -f1)"
    fi
}

# ── clean ──────────────────────────────────────
do_clean() {
    info "Cleaning..."
    rm -f tfr tfr_linux_amd64 tfr_linux_arm64 tfr_windows_amd64.exe tfr_darwin_amd64
    info "Done."
}

# ── main ───────────────────────────────────────
TARGET=${1:-all}

case "$TARGET" in
    clean)
        do_clean
        ;;
    linux)
        check_env; fetch_deps
        build_one linux amd64 tfr_linux_amd64
        build_one linux arm64 tfr_linux_arm64
        ;;
    windows)
        check_env; fetch_deps
        build_one windows amd64 tfr_windows_amd64.exe
        ;;
    darwin)
        check_env; fetch_deps
        build_one darwin amd64 tfr_darwin_amd64
        ;;
    all)
        check_env; fetch_deps
        build_one linux   amd64 tfr_linux_amd64
        build_one linux   arm64 tfr_linux_arm64
        build_one windows amd64 tfr_windows_amd64.exe
        build_one darwin  amd64 tfr_darwin_amd64
        echo ""
        info "All builds complete:"
        ls -lh tfr_linux_amd64 tfr_linux_arm64 tfr_windows_amd64.exe tfr_darwin_amd64 2>/dev/null || true
        ;;
    *)
        echo "Usage: $0 [all|linux|windows|darwin|clean]"
        exit 1
        ;;
esac
