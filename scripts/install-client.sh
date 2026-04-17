#!/bin/bash
# =============================================================================
#  PP Client Installer — https://github.com/vakaka1/pp
#  Назначение: только установить клиентские инструменты PP.
#  Подключение создаётся отдельно командой: pp-client-connect --config client.json
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

usage() {
    cat <<'EOF'
Использование: install-client.sh

Скрипт только устанавливает клиент PP:
- бинарник `pp`
- helper-команду `pp-client`
- helper-команду `pp-client-tun`
- helper-команду `pp-client-connect`
- GeoIP / GeoSite базы

Подключение создаётся отдельно:
  pp-client-connect --config client.json
  pp-client-connect tun --config client.json

Старые аргументы `--config`, `--socks5-port`, `--http-port` больше не поддерживаются.
EOF
}

if [ "$EUID" -eq 0 ]; then
    INSTALL_PREFIX="/usr/local"
    PP_DATA_DIR="/var/lib/pp-client"
    PP_CONFIG_DIR="/etc/pp-client"
else
    INSTALL_PREFIX="$HOME/.local"
    PP_DATA_DIR="$HOME/.local/share/pp-client"
    PP_CONFIG_DIR="$HOME/.config/pp-client"
fi

GITHUB_REPO="${GITHUB_REPO:-vakaka1/pp}"
RELEASE_TAG="${RELEASE_TAG:-latest}"
BIN_URL="${BIN_URL:-}"

PP_BIN="$INSTALL_PREFIX/bin/pp"
PP_RUNNER="$INSTALL_PREFIX/bin/pp-client"
PP_TUN_RUNNER="$INSTALL_PREFIX/bin/pp-client-tun"
PP_CONNECT="$INSTALL_PREFIX/bin/pp-client-connect"
PP_CONFIG="$PP_CONFIG_DIR/client.json"
PP_DATA_SUBDIR="$PP_DATA_DIR/data"
PP_TRANSPARENT_LISTEN="127.0.0.1:1090"
PP_INSTALL_UID="$(id -u)"
PP_INSTALL_GID="$(id -g)"

GEO_IP_URL="https://github.com/v2fly/geoip/releases/latest/download/geoip.dat"
GEO_SITE_URL="https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"

LEGACY_CONFIG=""
LEGACY_SOCKS5_PORT=""
LEGACY_HTTP_PORT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --config|-c)
            LEGACY_CONFIG="${2:-}"
            shift 2
            ;;
        --socks5-port)
            LEGACY_SOCKS5_PORT="${2:-}"
            shift 2
            ;;
        --http-port)
            LEGACY_HTTP_PORT="${2:-}"
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            die "Неизвестный аргумент: $1"
            ;;
    esac
done

if [ -n "$LEGACY_CONFIG" ] || [ -n "$LEGACY_SOCKS5_PORT" ] || [ -n "$LEGACY_HTTP_PORT" ]; then
    cat >&2 <<EOF
Скрипт install-client.sh больше не создаёт подключение.

Правильный сценарий:
  1. Установить клиент:
     curl -fsSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install-client.sh | bash
  2. Применить конфиг:
     pp-client-connect --config client.json

Порты теперь задаются только в самом клиентском JSON.
EOF
    exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *)       die "Архитектура $ARCH не поддерживается" ;;
esac
OS="linux"

info "Создание директорий..."
mkdir -p "$INSTALL_PREFIX/bin" "$PP_DATA_SUBDIR" "$PP_CONFIG_DIR"
ok "Директории созданы"

info "Загрузка бинарника pp ($OS/$GOARCH)..."

if [ -n "$BIN_URL" ]; then
    DOWNLOAD_URL="$BIN_URL"
else
    if [ "$RELEASE_TAG" = "latest" ]; then
        DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/pp_${OS}_${GOARCH}"
    else
        DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_TAG}/pp_${OS}_${GOARCH}"
    fi
fi

TMP_BIN="$(mktemp)"
if ! curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$TMP_BIN" "$DOWNLOAD_URL"; then
    rm -f "$TMP_BIN"
    die "Не удалось загрузить бинарник с $DOWNLOAD_URL"
fi

if ! file "$TMP_BIN" | grep -q "ELF"; then
    warn "Загруженный файл не является ELF-бинарником:"
    head -c 200 "$TMP_BIN" >&2
    rm -f "$TMP_BIN"
    die "Загрузка бинарника не удалась — проверьте DOWNLOAD_URL"
fi

install -m 755 "$TMP_BIN" "$PP_BIN"
rm -f "$TMP_BIN"
ok "Бинарник установлен: $PP_BIN ($("$PP_BIN" version 2>/dev/null || echo 'ok'))"

info "Загрузка GeoIP и GeoSite баз данных..."

GEOIP_FILE="$PP_DATA_SUBDIR/geoip.dat"
GEOSITE_FILE="$PP_DATA_SUBDIR/geosite.dat"

if [ -f "$GEOIP_FILE" ] && [ "$(stat -c%s "$GEOIP_FILE" 2>/dev/null || echo 0)" -gt 1000000 ]; then
    ok "geoip.dat уже существует — пропускаем"
else
    info "Загрузка geoip.dat (~23 MB)..."
    curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$GEOIP_FILE" "$GEO_IP_URL" \
        || { warn "Не удалось загрузить geoip.dat — geoip-правила будут отключены"; rm -f "$GEOIP_FILE"; }
fi

if [ -f "$GEOSITE_FILE" ] && [ "$(stat -c%s "$GEOSITE_FILE" 2>/dev/null || echo 0)" -gt 100000 ]; then
    ok "geosite.dat уже существует — пропускаем"
else
    info "Загрузка geosite.dat (~2 MB)..."
    curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$GEOSITE_FILE" "$GEO_SITE_URL" \
        || { warn "Не удалось загрузить geosite.dat — geosite-правила будут отключены"; rm -f "$GEOSITE_FILE"; }
fi
ok "Geo-данные готовы"

cat > "$PP_RUNNER" <<EOF
#!/bin/bash
set -euo pipefail

PP_BIN="$PP_BIN"
PP_DATA_DIR="$PP_DATA_DIR"
PP_CONFIG="$PP_CONFIG_DIR/client.json"

if [ ! -x "$PP_BIN" ]; then
    echo "pp binary not found: $PP_BIN" >&2
    exit 1
fi

if [ ! -f "$PP_CONFIG" ]; then
    echo "Client config not found: $PP_CONFIG" >&2
    echo "Run: pp-client-connect --config client.json" >&2
    exit 1
fi

cd "$PP_DATA_DIR"
exec "$PP_BIN" client --config "$PP_CONFIG" "$@"
EOF
chmod 755 "$PP_RUNNER"
ok "Команда запуска установлена: $PP_RUNNER"

cat > "$PP_TUN_RUNNER" <<EOF
#!/bin/bash
set -euo pipefail

PP_BIN="$PP_BIN"
PP_DATA_DIR="$PP_DATA_DIR"
PP_CONFIG="$PP_CONFIG_DIR/client.json"
PP_TRANSPARENT_LISTEN="$PP_TRANSPARENT_LISTEN"

if [ ! -x "$PP_BIN" ]; then
    echo "pp binary not found: $PP_BIN" >&2
    exit 1
fi

if [ ! -f "$PP_CONFIG" ]; then
    echo "Client config not found: $PP_CONFIG" >&2
    echo "Run: pp-client-connect tun --config client.json" >&2
    exit 1
fi

cd "$PP_DATA_DIR"
exec "$PP_BIN" client --config "$PP_CONFIG" --transparent-listen "$PP_TRANSPARENT_LISTEN" "$@"
EOF
chmod 755 "$PP_TUN_RUNNER"
ok "Команда full-tunnel запуска установлена: $PP_TUN_RUNNER"

cat > "$PP_CONNECT" <<'CONNECT'
#!/bin/bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

usage() {
    cat <<'EOF'
Использование:
  pp-client-connect --config client.json
  pp-client-connect tun --config client.json

Команда:
- сохраняет клиентский конфиг как активный
- в обычном режиме запускает `pp-client` в текущем терминале
- в режиме `tun` включает Linux system-wide TCP full-tunnel через transparent redirect
- в обоих режимах пишет логи прямо в текущий терминал до `Ctrl+C`

Чтобы переключить подключение, просто запустите команду ещё раз с другим JSON.
EOF
}

PP_BIN="__PP_BIN__"
PP_RUNNER="__PP_RUNNER__"
PP_TUN_RUNNER="__PP_TUN_RUNNER__"
PP_DATA_DIR="__PP_DATA_DIR__"
PP_CONFIG_DIR="__PP_CONFIG_DIR__"
PP_CONFIG="__PP_CONFIG__"
PP_DATA_SUBDIR="__PP_DATA_SUBDIR__"
PP_TRANSPARENT_LISTEN="127.0.0.1:1090"
PP_INSTALL_UID="__PP_INSTALL_UID__"
PP_INSTALL_GID="__PP_INSTALL_GID__"

CONFIG_FILE=""
MODE="proxy"

if [ "${1:-}" = "tun" ]; then
    MODE="tun"
    shift
fi

while [[ $# -gt 0 ]]; do
    case "$1" in
        --config|-c)
            CONFIG_FILE="${2:-}"
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            die "Неизвестный аргумент: $1"
            ;;
    esac
done

[ -n "$CONFIG_FILE" ] || die "Укажите --config client.json"
[ -f "$CONFIG_FILE" ] || die "Файл конфига не найден: $CONFIG_FILE"
[ -x "$PP_BIN" ] || die "pp не найден: $PP_BIN. Сначала выполните install-client.sh"
[ -x "$PP_RUNNER" ] || die "pp-client launcher не найден: $PP_RUNNER. Повторите установку клиента."
[ "$MODE" != "tun" ] || [ -x "$PP_TUN_RUNNER" ] || die "pp-client-tun launcher не найден: $PP_TUN_RUNNER. Повторите установку клиента."

ensure_paths() {
    if [ "$EUID" -eq 0 ]; then
        install -d -m 755 -o "$PP_INSTALL_UID" -g "$PP_INSTALL_GID" "$PP_DATA_DIR" "$PP_DATA_SUBDIR" "$PP_CONFIG_DIR"
    else
        mkdir -p "$PP_DATA_SUBDIR" "$PP_CONFIG_DIR"
    fi
}

write_active_config() {
    if [ -f "$PP_CONFIG" ]; then
        if [ "$EUID" -eq 0 ]; then
            install -m 600 -o "$PP_INSTALL_UID" -g "$PP_INSTALL_GID" "$PP_CONFIG" "$PP_CONFIG.bak"
        else
            install -m 600 "$PP_CONFIG" "$PP_CONFIG.bak"
        fi
    fi

    if [ "$EUID" -eq 0 ]; then
        install -m 600 -o "$PP_INSTALL_UID" -g "$PP_INSTALL_GID" "$CONFIG_FILE" "$PP_CONFIG"
    else
        install -m 600 "$CONFIG_FILE" "$PP_CONFIG"
    fi
}

cleanup_tun() {
    "$PP_BIN" full-tunnel down >/dev/null 2>&1 || true
}

ensure_paths

if ! VALIDATION_OUTPUT="$("$PP_BIN" validate-config --mode client --config "$CONFIG_FILE" 2>&1)"; then
    echo "$VALIDATION_OUTPUT" >&2
    die "Некорректный клиентский конфиг"
fi

if [ "$MODE" = "tun" ]; then
    [ "$EUID" -eq 0 ] || die "Режим tun требует root"
    command -v iptables >/dev/null 2>&1 || die "Для tun-режима требуется iptables"

    write_active_config
    ok "Активный конфиг full-tunnel обновлён: $PP_CONFIG"
    info "Поднимаю transparent redirect и запускаю pp-client-tun в foreground..."

    cleanup_tun
    trap cleanup_tun EXIT

    "$PP_BIN" full-tunnel up --config "$PP_CONFIG" --transparent-listen "$PP_TRANSPARENT_LISTEN" --owner root

    echo "  Режим:   tun (system-wide TCP full-tunnel)"
    echo "  Listen:  $PP_TRANSPARENT_LISTEN"
    echo "  Конфиг:  $PP_CONFIG"
    echo "  Логи:    в этом терминале"
    echo "  Стоп:    Ctrl+C"
    echo "  Примечание: UDP и IPv6 в этом режиме пока не туннелируются"
    echo ""
    "$PP_TUN_RUNNER"
else
    write_active_config
    ok "Активный конфиг proxy-режима обновлён: $PP_CONFIG"
    echo "  Режим:   proxy (SOCKS5/HTTP listeners)"
    echo "  Конфиг:  $PP_CONFIG"
    echo "  Логи:    в этом терминале"
    echo "  Стоп:    Ctrl+C"
    echo ""
    exec "$PP_RUNNER"
fi
CONNECT
sed -i \
    -e "s|__PP_BIN__|$PP_BIN|g" \
    -e "s|__PP_RUNNER__|$PP_RUNNER|g" \
    -e "s|__PP_TUN_RUNNER__|$PP_TUN_RUNNER|g" \
    -e "s|__PP_DATA_DIR__|$PP_DATA_DIR|g" \
    -e "s|__PP_CONFIG_DIR__|$PP_CONFIG_DIR|g" \
    -e "s|__PP_CONFIG__|$PP_CONFIG|g" \
    -e "s|__PP_DATA_SUBDIR__|$PP_DATA_SUBDIR|g" \
    -e "s|__PP_INSTALL_UID__|$PP_INSTALL_UID|g" \
    -e "s|__PP_INSTALL_GID__|$PP_INSTALL_GID|g" \
    "$PP_CONNECT"
chmod 755 "$PP_CONNECT"
ok "Команда подключения установлена: $PP_CONNECT"

echo ""
echo -e "${GREEN}============================================================${NC}"
echo -e "${GREEN}  PP Client установлен${NC}"
echo -e "${GREEN}============================================================${NC}"
echo ""
echo "  Бинарник:      $PP_BIN"
echo "  Данные:        $PP_DATA_DIR"
echo "  Geo-базы:      $PP_DATA_SUBDIR"
echo "  Launcher:      $PP_RUNNER"
echo "  Full-tunnel:   $PP_TUN_RUNNER"
echo "  Подключение:   $PP_CONNECT --config client.json"
echo "  Full-tunnel:   $PP_CONNECT tun --config client.json"
echo ""
if [[ ":$PATH:" != *":$INSTALL_PREFIX/bin:"* ]]; then
    warn "Каталог $INSTALL_PREFIX/bin не найден в PATH"
fi
echo -e "${GREEN}============================================================${NC}"
