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
- pp-client (основной бинарник)
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

PP_BIN="$INSTALL_PREFIX/bin/pp-client"
PP_CONFIG="$PP_CONFIG_DIR/client.json"
PP_DATA_SUBDIR="$PP_DATA_DIR/data"
PP_TRANSPARENT_LISTEN="127.0.0.1:1090"
PP_INSTALL_UID="$(id -u)"
PP_INSTALL_GID="$(id -g)"


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
    die "Параметры портов и конфига при установке больше не поддерживаются."
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
            DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/pp-client_${OS}_${GOARCH}"
        else
            DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION_TO_INSTALL}/pp-client_${OS}_${GOARCH}"
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


echo ""
echo -e "${BOLD}${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${GREEN}║                Установка клиента завершена!                  ║${NC}"
echo -e "${BOLD}${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${BOLD}Пути:${NC}"
echo -e "    Бинарник:    ${CYAN}$PP_BIN${NC}"
echo -e "    Данные:      ${CYAN}$PP_DATA_DIR${NC}"
echo ""
echo -e "  ${BOLD}Использование:${NC}"
echo -e "    Подключение:     ${CYAN}$PP_BIN start --config $PP_CONFIG${NC}"
echo -e "    Full-tunnel:     ${CYAN}sudo $PP_BIN full-tunnel up --config $PP_CONFIG${NC}"
echo ""
if [[ ":$PATH:" != *":$INSTALL_PREFIX/bin:"* ]]; then
    warn "Каталог $INSTALL_PREFIX/bin не найден в PATH. Возможно, потребуется добавить его."
fi
я добавить его."
fi
