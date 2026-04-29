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

build_pp_core() {
    info "Cleaning $BIN_DIR/pp-core"
    rm -f "$BIN_DIR/pp-core"

    info "Building pp-core"
    cd "$ROOT_DIR"
    if [ "$VERBOSE" = "true" ]; then
        GOCACHE="$GOCACHE_DIR" CGO_ENABLED=0 go build -v -ldflags="$LDFLAGS" -o "$BIN_DIR/pp-core" ./cmd/pp-core
    else
        GOCACHE="$GOCACHE_DIR" CGO_ENABLED=0 go build -ldflags="$LDFLAGS" -o "$BIN_DIR/pp-core" ./cmd/pp-core
    fi
    ok "Built $BIN_DIR/pp-core"
}

build_pp_client() {
    info "Cleaning $BIN_DIR/pp-client"
    rm -f "$BIN_DIR/pp-client"

    info "Building pp-client"
    cd "$ROOT_DIR"
    if [ "$VERBOSE" = "true" ]; then
        GOCACHE="$GOCACHE_DIR" CGO_ENABLED=0 go build -v -ldflags="$LDFLAGS" -o "$BIN_DIR/pp-client" ./cmd/pp-client
    else
        GOCACHE="$GOCACHE_DIR" CGO_ENABLED=0 go build -ldflags="$LDFLAGS" -o "$BIN_DIR/pp-client" ./cmd/pp-client
    fi
    ok "Built $BIN_DIR/pp-client"
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

BUILD_CORE=false
BUILD_CLIENT=false
build_web=false

if [ "$#" -eq 0 ]; then
    BUILD_CORE=true
    build_web=true
fi

for target in "$@"; do
    case "$target" in
        pp-core)
            BUILD_CORE=true
            ;;
        pp-client)
            BUILD_CLIENT=true
            ;;
        pp-web|web)
            build_web=true
            ;;
        all)
            BUILD_CORE=true
            BUILD_CLIENT=true
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

if [ "$BUILD_CORE" = "true" ]; then
    build_pp_core
    if systemctl is-active --quiet pp-core; then
        info "Restarting pp-core"
        sudo systemctl restart pp-core
        ok "Restarted pp-core"
    fi
fi

if [ "$BUILD_CLIENT" = "true" ]; then
    build_pp_client
fi

if [ "$build_web" = true ]; then
    build_pp_web
    if systemctl is-active --quiet pp-web; then
        info "Applying systemd changes for pp-web"
        # Пересоздаем файлы сервисов из инсталлера, если нужно, или просто перезапускаем
        # В данном случае, так как мы обновили install-server.sh, лучше запустить его часть или 
        # вручную обновить файлы. Но для rebuild.sh достаточно просто перезапуска, 
        # если вы уже один раз прогнали инсталлер.
        sudo systemctl restart pp-web
    fi
fi

ok "Rebuild and update complete"
