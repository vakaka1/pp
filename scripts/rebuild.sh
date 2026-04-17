#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
GOCACHE_DIR="${GOCACHE:-$ROOT_DIR/.cache/go-build}"

VERSION="${VERSION:-$(git -C "$ROOT_DIR" describe --tags --always 2>/dev/null || echo "dev")}"
DATE="${DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --verify HEAD 2>/dev/null || echo "none")}"
LDFLAGS="-s -w -X main.version=${VERSION} -X main.buildDate=${DATE} -X main.gitCommit=${COMMIT}"

info() {
    printf '[INFO] %s\n' "$*"
}

ok() {
    printf '[OK]   %s\n' "$*"
}

die() {
    printf '[ERROR] %s\n' "$*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage:
  bash scripts/rebuild.sh
  bash scripts/rebuild.sh pp
  bash scripts/rebuild.sh pp-web
  bash scripts/rebuild.sh pp pp-web

Behavior:
  - removes selected binaries from ./bin
  - rebuilds only the selected targets
  - keeps Go build cache intact for faster repeated rebuilds
EOF
}

build_pp() {
    info "Cleaning $BIN_DIR/pp"
    rm -f "$BIN_DIR/pp"

    info "Building pp"
    (
        cd "$ROOT_DIR"
        GOCACHE="$GOCACHE_DIR" CGO_ENABLED=0 go build -ldflags="$LDFLAGS" -o "$BIN_DIR/pp" ./cmd/pp
    )
    ok "Built $BIN_DIR/pp"
}

build_pp_web() {
    info "Cleaning $BIN_DIR/pp-web"
    rm -f "$BIN_DIR/pp-web"

    info "Building pp-web"
    (
        cd "$ROOT_DIR"
        GOCACHE="$GOCACHE_DIR" CGO_ENABLED=1 go build -ldflags="$LDFLAGS" -o "$BIN_DIR/pp-web" ./cmd/pp-web
    )
    ok "Built $BIN_DIR/pp-web"
}

mkdir -p "$BIN_DIR" "$GOCACHE_DIR"

build_core=false
build_web=false

if [ "$#" -eq 0 ]; then
    build_core=true
    build_web=true
fi

for target in "$@"; do
    case "$target" in
        pp)
            build_core=true
            ;;
        pp-web|web)
            build_web=true
            ;;
        all)
            build_core=true
            build_web=true
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            usage
            die "unknown target: $target"
            ;;
    esac
done

if [ "$build_core" = true ]; then
    build_pp
fi

if [ "$build_web" = true ]; then
    build_pp_web
fi
