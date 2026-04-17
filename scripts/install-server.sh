#!/bin/bash
# =============================================================================
#  PP Server Installer — https://github.com/vakaka1/pp
#  Запуск: curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-server.sh | sudo bash
# =============================================================================
set -euo pipefail

# ---------- Цвета ----------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }
step()  { echo -e "\n${BOLD}▶ $*${NC}"; }

# ---------- Проверка root ----------
[ "$EUID" -eq 0 ] || die "Запустите скрипт от root: curl ... | sudo bash"

# ---------- Конфигурация ----------
GITHUB_REPO="${GITHUB_REPO:-vakaka1/pp}"
RELEASE_TAG="${RELEASE_TAG:-latest}"

PP_USER="pp-server"
PP_BIN="/usr/local/bin/pp"
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

# ---------- Детектирование архитектуры ----------
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  GOARCH="amd64" ;;
  aarch64) GOARCH="arm64" ;;
  *)       die "Архитектура $ARCH не поддерживается" ;;
esac

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║     PP Server Installer — github.com/vakaka1/pp      ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════╝${NC}"
echo -e "  Архитектура: ${CYAN}linux/${GOARCH}${NC}"
echo ""

# =============================================================================
step "Установка системных зависимостей"
# =============================================================================
{
    if command -v apt-get &>/dev/null; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update -qq
        apt-get install -y -qq curl nginx certbot python3-certbot-nginx libcap2-bin
    elif command -v dnf &>/dev/null; then
        dnf install -y curl nginx certbot python3-certbot-nginx libcap
    elif command -v yum &>/dev/null; then
        yum install -y curl nginx certbot python3-certbot-nginx libcap
    fi
} >/dev/null 2>&1 &
pid=$!

width=20
pos=0
dir=1
while kill -0 "$pid" 2>/dev/null; do
    bar=""
    for ((i=0; i<width; i++)); do
        if [ "$i" -eq "$pos" ]; then
            bar="${bar}━"
        elif [ "$i" -eq "$((pos-1))" ] || [ "$i" -eq "$((pos+1))" ]; then
            bar="${bar}─"
        else
            bar="${bar}·"
        fi
    done
    printf "\r${GREEN}  [%s] Установка системных зависимостей...${NC}" "$bar"
    pos=$((pos + dir))
    if [ "$pos" -ge "$((width-1))" ]; then dir=-1; elif [ "$pos" -le 0 ]; then dir=1; fi
    sleep 0.08
done
wait "$pid"
printf "\r\033[K"

if ! command -v apt-get &>/dev/null && ! command -v dnf &>/dev/null && ! command -v yum &>/dev/null; then
    warn "Неизвестный пакетный менеджер — убедитесь что nginx, certbot и curl установлены"
fi
ok "Зависимости установлены"

# =============================================================================
step "Загрузка бинарников"
# =============================================================================

download_binary() {
    local name="$1"
    local dest="$2"
    local filename="${name}_linux_${GOARCH}"

    if [ "$RELEASE_TAG" = "latest" ]; then
        local url="https://github.com/${GITHUB_REPO}/releases/latest/download/${filename}"
    else
        local url="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_TAG}/${filename}"
    fi

    info "Загрузка ${name}..."
    local tmp
    tmp="$(mktemp)"
    if ! curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$tmp" "$url"; then
        rm -f "$tmp"
        die "Не удалось загрузить ${name} с ${url}"
    fi
    if ! file "$tmp" | grep -q "ELF"; then
        rm -f "$tmp"
        die "Загруженный файл ${name} не является исполняемым. Проверьте: ${url}"
    fi
    install -m 755 "$tmp" "$dest"
    rm -f "$tmp"
    ok "${name} → ${dest}"
}

download_binary "pp"     "$PP_BIN"
download_binary "pp-web" "$PP_WEB_BIN"

# Загружаем frontend dist для pp-web
FRONTEND_ARCHIVE_NAME="pp-web-frontend.tar.gz"
if [ "$RELEASE_TAG" = "latest" ]; then
    FRONTEND_URL="https://github.com/${GITHUB_REPO}/releases/latest/download/${FRONTEND_ARCHIVE_NAME}"
else
    FRONTEND_URL="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_TAG}/${FRONTEND_ARCHIVE_NAME}"
fi

info "Загрузка frontend для pp-web..."
mkdir -p "$PP_WEB_FRONTEND_DIR"
FRONTEND_TMP="$(mktemp)"
if curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "$FRONTEND_TMP" "$FRONTEND_URL"; then
    tar -xzf "$FRONTEND_TMP" -C "$PP_WEB_FRONTEND_DIR" --strip-components=1
    rm -f "$FRONTEND_TMP"
    chown -R "${PP_USER}:${PP_USER}" "$PP_WEB_FRONTEND_DIR"
    ok "Frontend pp-web → ${PP_WEB_FRONTEND_DIR}"
else
    rm -f "$FRONTEND_TMP"
    die "Не удалось загрузить frontend pp-web с ${FRONTEND_URL}"
fi

if command -v setcap &>/dev/null; then
    setcap 'cap_net_bind_service=+ep' "$PP_BIN" || true
fi

# =============================================================================
step "Создание пользователя и директорий"
# =============================================================================
id "$PP_USER" &>/dev/null || useradd -r -s /bin/false -d "$PP_CONFIG_DIR" "$PP_USER"

mkdir -p "$PP_CONFIG_DIR" "$PP_LOG_DIR" "$PP_DATA_DIR"
mkdir -p "$PP_GEO_DIR"
mkdir -p "$PP_WEB_DATA_DIR/generated" "$PP_WEB_DATA_DIR/certs"
mkdir -p "$PP_WEB_FRONTEND_DIR"
mkdir -p "$PP_NGINX_MANAGED_DIR"

chown -R "$PP_USER:$PP_USER" "$PP_CONFIG_DIR" "$PP_LOG_DIR" "$PP_DATA_DIR" "$PP_WEB_DATA_DIR" "$PP_NGINX_MANAGED_DIR"
chmod 750 "$PP_CONFIG_DIR"
ok "Пользователь ${PP_USER} и директории созданы"

if [ ! -f "$PP_NGINX_INCLUDE" ]; then
    cat > "$PP_NGINX_INCLUDE" <<EOF
include ${PP_NGINX_MANAGED_DIR}/*.conf;
EOF
fi
chmod 644 "$PP_NGINX_INCLUDE"
ok "Подготовлена директория для управляемых nginx-конфигов"

# =============================================================================
step "Загрузка GeoIP / GeoSite баз для server-side routing"
# =============================================================================
if ! curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "${PP_GEO_DIR}/geoip.dat" "$GEO_IP_URL"; then
    warn "Не удалось загрузить geoip.dat — server-side geoip правила не будут срабатывать"
fi
if ! curl -fsSL --connect-timeout 30 --retry 3 --retry-delay 2 -o "${PP_GEO_DIR}/geosite.dat" "$GEO_SITE_URL"; then
    warn "Не удалось загрузить geosite.dat — server-side geosite правила будут ограничены"
fi
chown -R "$PP_USER:$PP_USER" "$PP_GEO_DIR"
ok "Geo-данные подготовлены"

# =============================================================================
step "Разрешение привилегированных операций для pp-web"
# =============================================================================
SYSTEMCTL_BIN="$(command -v systemctl || echo /bin/systemctl)"
CERTBOT_BIN="$(command -v certbot || echo /usr/bin/certbot)"
NGINX_BIN="$(command -v nginx || echo /usr/sbin/nginx)"
cat > /etc/sudoers.d/pp-web <<EOF
Defaults:${PP_USER} !requiretty
${PP_USER} ALL=(root) NOPASSWD: ${SYSTEMCTL_BIN} restart pp-core, ${SYSTEMCTL_BIN} stop pp-core, ${SYSTEMCTL_BIN} start pp-core, ${SYSTEMCTL_BIN} stop nginx, ${SYSTEMCTL_BIN} start nginx, ${SYSTEMCTL_BIN} restart nginx, ${SYSTEMCTL_BIN} reload nginx, ${CERTBOT_BIN}, ${NGINX_BIN} -t
EOF
chmod 440 /etc/sudoers.d/pp-web
if command -v visudo &>/dev/null; then
    visudo -cf /etc/sudoers.d/pp-web >/dev/null || die "sudoers для pp-web не прошёл проверку"
fi
ok "pp-web может перезапускать pp-core, проверять nginx и выпускать сертификаты через sudo"

# =============================================================================
step "Создание systemd сервиса pp-core"
# =============================================================================
cat > /etc/systemd/system/pp-core.service <<EOF
[Unit]
Description=PP Core (proxy server)
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target nginx.service
Wants=network-online.target
# Запускается только когда pp-web уже сгенерировал конфиг
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

systemctl daemon-reload
systemctl enable pp-core
ok "Сервис pp-core создан и добавлен в автозагрузку (запустится после настройки в pp-web)"

# =============================================================================
step "Создание systemd сервиса pp-web (веб-панель управления)"
# =============================================================================
cat > /etc/systemd/system/pp-web.service <<EOF
[Unit]
Description=PP Web (management panel)
Documentation=https://github.com/${GITHUB_REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${PP_USER}
Group=${PP_USER}
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

# Отключаем ProtectSystem, так как pp-web управляет nginx и certbot через sudo, 
# что требует записи в различные системные директории (/etc/nginx, /var/log, /run и т.д.)
ProtectSystem=off
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=${PP_LOG_DIR} ${PP_WEB_DATA_DIR} ${PP_CONFIG_DIR} ${PP_NGINX_MANAGED_DIR} /etc/letsencrypt /run /var/lib/nginx /var/cache/nginx /var/log/nginx

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable pp-web
systemctl start pp-web \
    && ok "pp-web запущен" \
    || warn "pp-web не запустился — проверьте: journalctl -u pp-web -n 30"

# =============================================================================
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║              Установка завершена!                    ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  Откройте веб-панель управления и завершите настройку:"
echo ""

# Определяем внешний IP для удобства
SERVER_IP="$(curl -fsSL --connect-timeout 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
echo -e "  ${CYAN}${BOLD}http://${SERVER_IP}:4090${NC}"
echo ""
echo -e "  Там вы сможете:"
echo -e "    • Создать подключение (домен, ключи — всё через интерфейс)"
echo -e "    • Выпустить SSL-сертификат"
echo -e "    • Скачать готовый конфиг для клиента"
echo -e "    • Запустить и перезапустить pp-core"
echo ""
echo -e "  Логи: ${CYAN}journalctl -u pp-web -f${NC}"
echo ""
