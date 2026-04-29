#!/bin/bash
# =============================================================================
#  PP Server Installer — https://github.com/vakaka1/pp
#  Запуск: curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-server.sh | bash
#          если вы уже root; иначе: ... | sudo bash
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

# ---------- Проверка root ----------
[ "$EUID" -eq 0 ] || die "Запустите скрипт от root. Если sudo доступен: curl ... | sudo bash"

# ---------- Конфигурация ----------
SCRIPT_SOURCE="${BASH_SOURCE:-}"
if [ -n "$SCRIPT_SOURCE" ] && [ -f "$SCRIPT_SOURCE" ]; then
    ROOT_DIR="$(cd "$(dirname "$SCRIPT_SOURCE")/.." && pwd)"
else
    ROOT_DIR=""
fi
GITHUB_REPO="${GITHUB_REPO:-vakaka1/pp}"
RELEASE_TAG="${RELEASE_TAG:-latest}"

PP_USER="pp-server"
PP_BIN="/usr/local/bin/pp-core"
PP_WEB_BIN="/usr/local/bin/pp-web"

PP_CONFIG_DIR="/etc/pp"
PP_LOG_DIR="/var/log/pp"
PP_DATA_DIR="/var/lib/pp"
PP_GEO_DIR="${PP_DATA_DIR}/data"
PP_WEB_DATA_DIR="/var/lib/pp-web"
PP_WEB_FRONTEND_DIR="/usr/share/pp-web/frontend"
PP_NGINX_MANAGED_DIR="/etc/nginx/pp-sites"
PP_NGINX_INCLUDE="/etc/nginx/conf.d/pp-managed.conf"

PP_WEB_LISTEN="0.0.0.0:4090"
GEO_IP_URL="https://github.com/v2fly/geoip/releases/latest/download/geoip.dat"
GEO_SITE_URL="https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"
PP_WEB_SERVICE_USER="$PP_USER"
PP_WEB_SERVICE_GROUP="$PP_USER"

# ---------- Детектирование архитектуры ----------
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *)       die "Архитектура $ARCH не поддерживается" ;;
esac

clear
echo ""
echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${CYAN}║                 PP Server Installer                  ║${NC}"
echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════╝${NC}"
echo -e "  GitHub:      ${CYAN}github.com/${GITHUB_REPO}${NC}"
echo -e "  Архитектура: ${CYAN}linux/${GOARCH}${NC}"
echo ""

# Определение версии
info "Получение информации о релизе..."
if [ "$RELEASE_TAG" = "latest" ]; then
    LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name":' | head -n 1 | sed -E 's/.*"([^"]+)".*/\1/' || echo "latest")
    VERSION_TO_INSTALL="$LATEST_TAG"
else
    VERSION_TO_INSTALL="$RELEASE_TAG"
fi
ok "Версия для установки: ${BOLD}${VERSION_TO_INSTALL}${NC}"

# =============================================================================
step "Установка системных зависимостей"
# =============================================================================
install_deps() {
    if command -v apt-get &>/dev/null; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update -qq
        apt-get install -y -qq curl nginx certbot python3-certbot-nginx libcap2-bin
    elif command -v dnf &>/dev/null; then
        dnf install -y curl nginx certbot python3-certbot-nginx libcap
    elif command -v yum &>/dev/null; then
        yum install -y curl nginx certbot python3-certbot-nginx libcap
    else
        return 1
    fi
}
run_with_spinner "Установка системных пакетов (nginx, certbot, curl, libcap)" install_deps || warn "Пакетный менеджер не найден или ошибка установки — убедитесь, что зависимости установлены"

# =============================================================================
step "Загрузка бинарников и компонентов"
# =============================================================================

download_binary() {
    local name="$1"
    local dest="$2"
    local filename="${name}_linux_${GOARCH}"

    if [ -n "$ROOT_DIR" ] && [ -f "$ROOT_DIR/bin/$name" ]; then
        install -m 755 "$ROOT_DIR/bin/$name" "$dest"
        return 0
    fi

    if [ "$VERSION_TO_INSTALL" = "latest" ]; then
        local url="https://github.com/${GITHUB_REPO}/releases/latest/download/${filename}"
    else
        local url="https://github.com/${GITHUB_REPO}/releases/download/${VERSION_TO_INSTALL}/${filename}"
    fi

    local tmp
    tmp="$(mktemp)"
    if ! curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$tmp" "$url"; then
        rm -f "$tmp"
        return 1
    fi
    if ! file "$tmp" | grep -q "ELF"; then
        rm -f "$tmp"
        return 1
    fi
    install -m 755 "$tmp" "$dest"
    rm -f "$tmp"
}

run_with_spinner "Загрузка pp-core" download_binary "pp-core" "$PP_BIN"
run_with_spinner "Загрузка pp-web (панель)" download_binary "pp-web" "$PP_WEB_BIN"

download_frontend() {
    local arch_name="pp-web-frontend.tar.gz"
    if [ "$VERSION_TO_INSTALL" = "latest" ]; then
        local url="https://github.com/${GITHUB_REPO}/releases/latest/download/${arch_name}"
    else
        local url="https://github.com/${GITHUB_REPO}/releases/download/${VERSION_TO_INSTALL}/${arch_name}"
    fi

    mkdir -p "$PP_WEB_FRONTEND_DIR"
    local tmp="$(mktemp)"
    if curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$tmp" "$url"; then
        tar -xzf "$tmp" -C "$PP_WEB_FRONTEND_DIR" --strip-components=1
        rm -f "$tmp"
        return 0
    else
        rm -f "$tmp"
        return 1
    fi
}
run_with_spinner "Загрузка frontend-ресурсов pp-web" download_frontend

if command -v setcap &>/dev/null; then
    setcap 'cap_net_bind_service=+ep' "$PP_BIN" || true
fi

# Вывод версии
INSTALLED_VER=$("$PP_BIN" version 2>/dev/null || echo "неизвестно")
ok "Установлена версия ядра: ${CYAN}${INSTALLED_VER}${NC}"

# =============================================================================
step "Настройка окружения и данных"
# =============================================================================
setup_env() {
    id "$PP_USER" &>/dev/null || useradd -r -s /bin/false -d "$PP_CONFIG_DIR" "$PP_USER"

    mkdir -p "$PP_CONFIG_DIR" "$PP_LOG_DIR" "$PP_DATA_DIR" "$PP_GEO_DIR"
    mkdir -p "$PP_WEB_DATA_DIR/generated" "$PP_WEB_DATA_DIR/certs"
    mkdir -p "$PP_WEB_FRONTEND_DIR" "$PP_NGINX_MANAGED_DIR"

    chown -R "$PP_USER:$PP_USER" "$PP_CONFIG_DIR" "$PP_LOG_DIR" "$PP_DATA_DIR" "$PP_WEB_DATA_DIR" "$PP_NGINX_MANAGED_DIR"
    chmod 750 "$PP_CONFIG_DIR"

    if [ ! -f "$PP_NGINX_INCLUDE" ]; then
        echo "include ${PP_NGINX_MANAGED_DIR}/*.conf;" > "$PP_NGINX_INCLUDE"
    fi
    chmod 644 "$PP_NGINX_INCLUDE"
}
run_with_spinner "Создание системного пользователя и директорий" setup_env

download_geo() {
    curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "${PP_GEO_DIR}/geoip.dat" "$GEO_IP_URL" || true
    curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "${PP_GEO_DIR}/geosite.dat" "$GEO_SITE_URL" || true
    chown -R "$PP_USER:$PP_USER" "$PP_GEO_DIR"
}
run_with_spinner "Загрузка баз GeoIP и GeoSite" download_geo

setup_sudoers() {
    if command -v sudo &>/dev/null; then
        local sys_bin="$(command -v systemctl || echo /bin/systemctl)"
        local crt_bin="$(command -v certbot || echo /usr/bin/certbot)"
        cat > /etc/sudoers.d/pp-web <<EOF
Defaults:${PP_USER} !requiretty
${PP_USER} ALL=(root) NOPASSWD: ${sys_bin} restart pp-core, ${sys_bin} stop pp-core, ${sys_bin} start pp-core, ${sys_bin} stop nginx, ${sys_bin} start nginx, ${sys_bin} restart nginx, ${sys_bin} reload nginx, ${sys_bin} --no-block start pp-web-update, ${crt_bin}
EOF
        chmod 440 /etc/sudoers.d/pp-web
        if command -v visudo &>/dev/null; then
            visudo -cf /etc/sudoers.d/pp-web >/dev/null || return 1
        fi
        return 0
    else
        return 1
    fi
}
if run_with_spinner "Настройка привилегий для веб-панели (sudoers)" setup_sudoers; then
    :
else
    PP_WEB_SERVICE_USER="root"
    PP_WEB_SERVICE_GROUP="root"
    warn "sudo не найден: pp-web будет работать от имени root"
fi

# =============================================================================
step "Регистрация системных служб"
# =============================================================================

setup_services() {
    cat > /etc/systemd/system/pp-core.service <<EOF
[Unit]
Description=PP Core (proxy server)
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target nginx.service
Wants=network-online.target
ConditionPathExists=${PP_WEB_DATA_DIR}/generated/pp-core.json

[Service]
Type=simple
User=${PP_USER}
Group=${PP_USER}
WorkingDirectory=${PP_DATA_DIR}
ExecStart=${PP_BIN} core --config ${PP_WEB_DATA_DIR}/generated/pp-core.json
Restart=always
RestartSec=5
TimeoutStopSec=30
LimitNOFILE=1048576

StandardOutput=append:${PP_LOG_DIR}/pp-core.log
StandardError=append:${PP_LOG_DIR}/pp-core.log

ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=${PP_LOG_DIR} ${PP_CONFIG_DIR} ${PP_DATA_DIR} ${PP_WEB_DATA_DIR}
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

    cat > /etc/systemd/system/pp-web-update.service <<EOF
[Unit]
Description=PP Web update job
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
Group=root
ExecStart=${PP_WEB_BIN} \\
    apply-release \\
    --repo ${GITHUB_REPO} \\
    --tag latest \\
    --pp-path ${PP_BIN} \\
    --pp-web-path ${PP_WEB_BIN} \\
    --frontend-dist ${PP_WEB_FRONTEND_DIR} \\
    --status-path ${PP_WEB_DATA_DIR}/update-status.json \\
    --pp-service pp-core \\
    --web-service pp-web
EOF

    cat > /etc/systemd/system/pp-web.service <<EOF
[Unit]
Description=PP Web (management panel)
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${PP_WEB_SERVICE_USER}
Group=${PP_WEB_SERVICE_GROUP}
WorkingDirectory=${PP_WEB_DATA_DIR}
ExecStart=${PP_WEB_BIN} \\
    --listen ${PP_WEB_LISTEN} \\
    --db ${PP_WEB_DATA_DIR}/pp-web.sqlite \\
    --core-config ${PP_WEB_DATA_DIR}/generated/pp-core.json \\
    --frontend-dist ${PP_WEB_FRONTEND_DIR} \\
    --project-root ${PP_DATA_DIR}
Restart=always
RestartSec=5
TimeoutStopSec=30

StandardOutput=append:${PP_LOG_DIR}/pp-web.log
StandardError=append:${PP_LOG_DIR}/pp-web.log

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable pp-core >/dev/null 2>&1
    systemctl enable pp-web >/dev/null 2>&1
}
run_with_spinner "Создание и включение systemd сервисов (pp-core, pp-web)" setup_services

start_web() {
    if systemctl is-active --quiet pp-web; then
        systemctl restart pp-web || return 1
    else
        systemctl start pp-web || return 1
    fi
}
run_with_spinner "Запуск веб-панели управления" start_web || warn "pp-web не запустился, проверьте логи: journalctl -u pp-web -n 30"

# =============================================================================
echo ""
echo -e "${BOLD}${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${GREEN}║                Установка сервера завершена!                  ║${NC}"
echo -e "${BOLD}${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  Откройте веб-панель управления для завершения настройки:"
echo ""

SERVER_IP="$(curl -fsSL --connect-timeout 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
echo -e "  ${CYAN}${BOLD}http://${SERVER_IP}:4090${NC}"
echo ""
echo -e "  ${BOLD}В панели вы сможете:${NC}"
echo -e "    • Создать подключение и выпустить SSL-сертификат"
echo -e "    • Скачать готовый конфиг для клиента"
echo -e "    • Запустить основной сервер ${CYAN}pp-core${NC}"
echo ""
echo -e "  ${BOLD}Полезные команды:${NC}"
echo -e "    Логи сервера:  ${CYAN}journalctl -u pp-core -f${NC}"
echo -e "    Логи панели:   ${CYAN}journalctl -u pp-web -f${NC}"
echo ""
