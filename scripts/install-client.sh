#!/bin/bash
# =============================================================================
#  PP Client Installer — https://github.com/vakaka1/pp
# =============================================================================
set -euo pipefail

# ---------- Цвета и оформление ----------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

info()  { echo -e "${CYAN}ℹ${NC} $*"; }
ok()    { echo -e "${GREEN}✔${NC} $*"; }
warn()  { echo -e "${YELLOW}⚠${NC} $*"; }
die()   { echo -e "${RED}✖${NC} $*" >&2; exit 1; }
step()  { echo -e "\n${BOLD}${BLUE}▶${NC} ${BOLD}$*${NC}"; }

run_with_spinner() {
    local msg="$1"
    shift
    local pid
    local temp_log
    temp_log=$(mktemp)
    
    "$@" >"$temp_log" 2>&1 &
    pid=$!

    local frames=("⠋" "⠙" "⠹" "⠸" "⠼" "⠴" "⠦" "⠧" "⠇" "⠏")
    local i=0
    while kill -0 "$pid" 2>/dev/null; do
        printf "\r\033[K${CYAN}%s${NC} %s" "${frames[i]}" "$msg"
        i=$(((i + 1) % 10))
        sleep 0.1
    done
    
    wait "$pid"
    local exit_code=$?
    
    printf "\r\033[K"
    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}✔${NC} ${msg}"
    else
        echo -e "${RED}✖${NC} ${msg} (ошибка)"
        echo -e "${DIM}" >&2
        cat "$temp_log" >&2
        echo -e "${NC}" >&2
        rm -f "$temp_log"
        exit $exit_code
    fi
    rm -f "$temp_log"
}

usage() {
    cat <<'USAGE'
Использование: install-client.sh

Скрипт устанавливает клиентские инструменты PP:
- pp (основной бинарник)
- pp-client (запуск обычного режима)
- pp-client-tun (запуск режима full-tunnel)
- pp-client-connect (управление конфигурацией)
- GeoIP / GeoSite базы
USAGE
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
        --config|-c) LEGACY_CONFIG="${2:-}"; shift 2 ;;
        --socks5-port) LEGACY_SOCKS5_PORT="${2:-}"; shift 2 ;;
        --http-port) LEGACY_HTTP_PORT="${2:-}"; shift 2 ;;
        --help|-h) usage; exit 0 ;;
        *) die "Неизвестный аргумент: $1" ;;
    esac
done

if [ -n "$LEGACY_CONFIG" ] || [ -n "$LEGACY_SOCKS5_PORT" ] || [ -n "$LEGACY_HTTP_PORT" ]; then
    die "Параметры портов и конфига при установке больше не поддерживаются.\nИспользуйте pp-client-connect --config client.json после установки."
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *)       die "Архитектура $ARCH не поддерживается" ;;
esac
OS="linux"

clear
echo ""
echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${CYAN}║                 PP Client Installer                  ║${NC}"
echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════╝${NC}"
echo -e "  GitHub:      ${CYAN}github.com/${GITHUB_REPO}${NC}"
echo -e "  Архитектура: ${CYAN}${OS}/${GOARCH}${NC}"
echo ""

# Определение версии
info "Получение информации о релизе..."
if [ -n "$BIN_URL" ]; then
    VERSION_TO_INSTALL="custom"
else
    if [ "$RELEASE_TAG" = "latest" ]; then
        LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name":' | head -n 1 | sed -E 's/.*"([^"]+)".*/\1/' || echo "latest")
        VERSION_TO_INSTALL="$LATEST_TAG"
    else
        VERSION_TO_INSTALL="$RELEASE_TAG"
    fi
fi
ok "Версия для установки: ${BOLD}${VERSION_TO_INSTALL}${NC}"

step "Подготовка файловой системы"
create_dirs() {
    mkdir -p "$INSTALL_PREFIX/bin" "$PP_DATA_SUBDIR" "$PP_CONFIG_DIR"
}
run_with_spinner "Создание директорий ($INSTALL_PREFIX, $PP_DATA_DIR)" create_dirs

step "Загрузка компонентов"

download_pp() {
    if [ -n "$BIN_URL" ]; then
        DOWNLOAD_URL="$BIN_URL"
    else
        if [ "$VERSION_TO_INSTALL" = "latest" ]; then
            DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/pp_${OS}_${GOARCH}"
        else
            DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION_TO_INSTALL}/pp_${OS}_${GOARCH}"
        fi
    fi

    local tmp
    tmp="$(mktemp)"
    curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$tmp" "$DOWNLOAD_URL" || return 1
    if ! file "$tmp" | grep -q "ELF"; then
        return 1
    fi
    install -m 755 "$tmp" "$PP_BIN"
    rm -f "$tmp"
}

run_with_spinner "Загрузка бинарника pp" download_pp

# Вывод версии установленного бинарника
INSTALLED_VER=$("$PP_BIN" version 2>/dev/null || echo "неизвестно")
ok "Установлена версия: ${CYAN}${INSTALLED_VER}${NC}"

download_geo() {
    local geoip_file="$PP_DATA_SUBDIR/geoip.dat"
    local geosite_file="$PP_DATA_SUBDIR/geosite.dat"

    if ! [ -f "$geoip_file" ] || [ "$(stat -c%s "$geoip_file" 2>/dev/null || echo 0)" -lt 1000000 ]; then
        curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$geoip_file" "$GEO_IP_URL" || return 1
    fi

    if ! [ -f "$geosite_file" ] || [ "$(stat -c%s "$geosite_file" 2>/dev/null || echo 0)" -lt 100000 ]; then
        curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$geosite_file" "$GEO_SITE_URL" || return 1
    fi
}

run_with_spinner "Загрузка GeoIP / GeoSite баз" download_geo

step "Настройка скриптов запуска"

setup_scripts() {
    cat > "$PP_RUNNER" <<RUNNER_EOF
#!/bin/bash
set -euo pipefail

PP_BIN="$PP_BIN"
PP_DATA_DIR="$PP_DATA_DIR"
PP_CONFIG="$PP_CONFIG_DIR/client.json"

if [ ! -x "\$PP_BIN" ]; then
    echo "pp binary not found: \$PP_BIN" >&2
    exit 1
fi

if [ ! -f "\$PP_CONFIG" ]; then
    echo "Client config not found: \$PP_CONFIG" >&2
    echo "Run: pp-client-connect --config client.json" >&2
    exit 1
fi

cd "\$PP_DATA_DIR"
exec "\$PP_BIN" client --config "\$PP_CONFIG" "\$@"
RUNNER_EOF
    chmod 755 "$PP_RUNNER"

    cat > "$PP_TUN_RUNNER" <<TUN_RUNNER_EOF
#!/bin/bash
set -euo pipefail

PP_BIN="$PP_BIN"
PP_DATA_DIR="$PP_DATA_DIR"
PP_CONFIG="$PP_CONFIG_DIR/client.json"
PP_TRANSPARENT_LISTEN="$PP_TRANSPARENT_LISTEN"

if [ ! -x "\$PP_BIN" ]; then
    echo "pp binary not found: \$PP_BIN" >&2
    exit 1
fi

if [ ! -f "\$PP_CONFIG" ]; then
    echo "Client config not found: \$PP_CONFIG" >&2
    echo "Run: pp-client-connect tun --config client.json" >&2
    exit 1
fi

cd "\$PP_DATA_DIR"
exec "\$PP_BIN" client --config "\$PP_CONFIG" --transparent-listen "\$PP_TRANSPARENT_LISTEN" "\$@"
TUN_RUNNER_EOF
    chmod 755 "$PP_TUN_RUNNER"

    cat > "$PP_CONNECT" <<'CONNECT'
#!/bin/bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}ℹ${NC} $*"; }
ok()    { echo -e "${GREEN}✔${NC} $*"; }
warn()  { echo -e "${YELLOW}⚠${NC} $*"; }
die()   { echo -e "${RED}✖${NC} $*" >&2; exit 1; }

usage() {
    cat <<'USAGE_EOF'
Использование:
  pp-client-connect --config client.json
  pp-client-connect tun --config client.json

Команда:
- сохраняет клиентский конфиг как активный
- в обычном режиме запускает `pp-client` в текущем терминале
- в режиме `tun` включает Linux system-wide TCP full-tunnel
USAGE_EOF
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
        --config|-c) CONFIG_FILE="${2:-}"; shift 2 ;;
        --help|-h) usage; exit 0 ;;
        *) die "Неизвестный аргумент: $1" ;;
    esac
done

[ -n "$CONFIG_FILE" ] || die "Укажите --config client.json"
[ -f "$CONFIG_FILE" ] || die "Файл конфига не найден: $CONFIG_FILE"
[ -x "$PP_BIN" ] || die "Бинарник pp не найден: $PP_BIN"

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
    ok "Конфиг full-tunnel обновлён"
    info "Поднимаю transparent redirect..."

    cleanup_tun
    trap cleanup_tun EXIT

    "$PP_BIN" full-tunnel up --config "$PP_CONFIG" --transparent-listen "$PP_TRANSPARENT_LISTEN" --owner root

    echo -e "\n  ${CYAN}Режим:${NC}   tun (system-wide TCP full-tunnel)"
    echo -e "  ${CYAN}Listen:${NC}  $PP_TRANSPARENT_LISTEN"
    echo -e "  ${CYAN}Конфиг:${NC}  $PP_CONFIG"
    echo -e "  ${CYAN}Стоп:${NC}    Ctrl+C\n"
    "$PP_TUN_RUNNER"
else
    write_active_config
    ok "Конфиг proxy-режима обновлён"
    echo -e "\n  ${CYAN}Режим:${NC}   proxy (SOCKS5/HTTP listeners)"
    echo -e "  ${CYAN}Конфиг:${NC}  $PP_CONFIG"
    echo -e "  ${CYAN}Стоп:${NC}    Ctrl+C\n"
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
}

run_with_spinner "Генерация лаунчеров (pp-client, pp-client-tun, pp-client-connect)" setup_scripts

echo ""
echo -e "${BOLD}${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${GREEN}║                Установка клиента завершена!                  ║${NC}"
echo -e "${BOLD}${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${BOLD}Пути:${NC}"
echo -e "    Бинарник:    ${CYAN}$PP_BIN${NC}"
echo -e "    Данные:      ${CYAN}$PP_DATA_DIR${NC}"
echo -e "    Geo-базы:    ${CYAN}$PP_DATA_SUBDIR${NC}"
echo ""
echo -e "  ${BOLD}Использование:${NC}"
echo -e "    Подключение:     ${CYAN}$PP_CONNECT --config client.json${NC}"
echo -e "    Full-tunnel:     ${CYAN}$PP_CONNECT tun --config client.json${NC}"
echo ""
if [[ ":$PATH:" != *":$INSTALL_PREFIX/bin:"* ]]; then
    warn "Каталог $INSTALL_PREFIX/bin не найден в PATH. Возможно, потребуется добавить его."
fi
