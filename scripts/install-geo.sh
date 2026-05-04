#!/bin/bash
set -euo pipefail

# =============================================================================
#  PP Geo-Database Installer — https://github.com/vakaka1/pp
# =============================================================================

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

# Определение директории установки
if [ "$EUID" -eq 0 ]; then
    if [ -d "/var/lib/pp/data" ]; then
        PP_DATA_DIR="/var/lib/pp/data"
    else
        PP_DATA_DIR="/var/lib/pp-client/data"
    fi
else
    PP_DATA_DIR="$HOME/.local/share/pp-client/data"
fi

GEO_IP_URL="https://github.com/v2fly/geoip/releases/latest/download/geoip.dat"
GEO_SITE_URL="https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"
GEO_IP_SUM_URL="${GEO_IP_URL}.sha256sum"
GEO_SITE_SUM_URL="${GEO_SITE_URL}.sha256sum"

geoip_file="$PP_DATA_DIR/geoip.dat"
geosite_file="$PP_DATA_DIR/geosite.dat"

clear
echo ""
echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${CYAN}║               PP Geo-Database Installer              ║${NC}"
echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════╝${NC}"
echo ""
info "Целевая директория: ${PP_DATA_DIR}"
mkdir -p "$PP_DATA_DIR"

check_geoip() {
    remote_ip_sum=$(curl -fsSL "$GEO_IP_SUM_URL" | awk '{print $1}' || echo "")
    local_ip_sum=""
    if [ -f "$geoip_file" ]; then
        local_ip_sum=$(sha256sum "$geoip_file" | awk '{print $1}')
    fi

    if [ -n "$remote_ip_sum" ] && [ "$remote_ip_sum" = "$local_ip_sum" ]; then
        return 0 # Не нужно обновлять
    else
        return 1 # Нужно обновить
    fi
}

check_geosite() {
    remote_site_sum=$(curl -fsSL "$GEO_SITE_SUM_URL" | awk '{print $1}' || echo "")
    local_site_sum=""
    if [ -f "$geosite_file" ]; then
        local_site_sum=$(sha256sum "$geosite_file" | awk '{print $1}')
    fi

    if [ -n "$remote_site_sum" ] && [ "$remote_site_sum" = "$local_site_sum" ]; then
        return 0
    else
        return 1
    fi
}

step "Проверка GeoIP базы..."
update_ip=0
if check_geoip; then
    ok "GeoIP база уже актуальна."
else
    update_ip=1
    info "Требуется обновление GeoIP базы."
fi

step "Проверка GeoSite базы..."
update_site=0
if check_geosite; then
    ok "GeoSite база уже актуальна."
else
    update_site=1
    info "Требуется обновление GeoSite базы."
fi

download_geoip() {
    curl -fsSL --connect-timeout 30 --retry 3 -o "${geoip_file}.tmp" "$GEO_IP_URL" || return 1
    mv "${geoip_file}.tmp" "$geoip_file"
}

download_geosite() {
    curl -fsSL --connect-timeout 30 --retry 3 -o "${geosite_file}.tmp" "$GEO_SITE_URL" || return 1
    mv "${geosite_file}.tmp" "$geosite_file"
}

if [ "$update_ip" -eq 1 ]; then
    run_with_spinner "Загрузка GeoIP базы" download_geoip
fi

if [ "$update_site" -eq 1 ]; then
    run_with_spinner "Загрузка GeoSite базы" download_geosite
fi

echo ""
echo -e "${BOLD}${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${GREEN}║                Установка баз завершена!                      ║${NC}"
echo -e "${BOLD}${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
