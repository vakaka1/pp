# Конфигурация клиента

Конфигурационный файл клиента — это JSON, который содержит параметры сервера и настройки локальных сервисов.

## Расположение конфигов

| Платформа | Путь | Создаётся |
|-----------|------|-----------|
| Linux (root) | `/etc/pp-client/*.json` | `install-client.sh` или `pp-client import` |
| Linux (user) | `~/.config/pp-client/*.json` или `configs/*.json` | `pp-client import` |
| Windows | `%APPDATA%\pp\*.json` | `install-client.ps1` или `pp-client import` |

## Пример структуры

```json
{
  "meta": {
    "client_name": "my-client",
    "protocol": "pp-fallback",
    "generated_at": "2026-04-30T09:00:00Z"
  },
  "log": {
    "level": "info",
    "output": "stdout"
  },
  "client": {
    "socks5_listen": "127.0.0.1:1080",
    "http_proxy_listen": "127.0.0.1:8080",
    "server": {
      "address": "your-domain.com:443",
      "domain": "your-domain.com",
      "noise_public_key": "КЛЮЧ_ИЗ_ПАНЕЛИ",
      "psk": "КЛЮЧ_АУТЕНТИФИКАЦИИ",
      "tls_fingerprint": "chrome",
      "grpc_path": "/pp.v1.TunnelService/Connect",
      "grpc_user_agent": "grpc-go/1.62.1"
    },
    "transport": {
      "shaper_enabled": true,
      "jitter_max_ms": 30,
      "keepalive_interval_seconds": 25,
      "reconnect_stream_min": 800,
      "reconnect_stream_max": 1200,
      "reconnect_duration_min_h": 3,
      "reconnect_duration_max_h": 5
    },
    "connection_pool": {
      "size": 1
    }
  }
}
```

## Описание параметров

### Секция `meta` (информационная)
Заполняется автоматически при импорте через `ppf://` URI. `pp-client` не использует эти поля для работы — они нужны для GUI.
*   **client_name**: Имя клиента.
*   **protocol**: Протокол подключения (`pp-fallback`).
*   **generated_at**: Время генерации конфига (ISO 8601).

### Секция `log`
*   **level**: Уровень логирования — `info` или `debug`.
*   **output**: Куда писать логи — `stdout`.

### Секция `client`
*   **socks5_listen**: Адрес SOCKS5 прокси (по умолчанию `127.0.0.1:1080`).
*   **http_proxy_listen**: Адрес HTTP прокси (по умолчанию `127.0.0.1:8080`). На Windows с `--system-proxy` этот адрес записывается в реестр.
*   **transparent_listen**: (Опционально, только Linux) Порт для прозрачного проксирования (используется с `full-tunnel up`).

### Секция `client.server`
*   **address**: IP или домен сервера с портом (обычно 443).
*   **domain**: Домен для проверки SSL-сертификата.
*   **noise_public_key**: Публичный ключ сервера (Noise). Генерируется на сервере.
*   **psk**: Личный ключ доступа клиента (Pre-Shared Key).
*   **grpc_path**: Секретный путь к gRPC-сервису. Должен совпадать с настройками сервера.
*   **grpc_user_agent**: User-Agent для gRPC-запросов (для маскировки).
*   **tls_fingerprint**: (Опционально) Имитация отпечатка браузера (`chrome`, `firefox`, `safari`).

### Секция `client.transport`
*   **shaper_enabled**: Включает сглаживание трафика для защиты от анализа по паттернам.
*   **jitter_max_ms**: Максимальная задержка (мс) для имитации естественного поведения.
*   **keepalive_interval_seconds**: Интервал keepalive (сек).
*   **reconnect_stream_min / max**: Диапазон для автоматического пересоздания стримов (число запросов).
*   **reconnect_duration_min_h / max_h**: Диапазон для пересоздания всего соединения (часы).

### Секция `client.connection_pool`
*   **size**: Количество параллельных соединений к серверу (по умолчанию 1).

## Маршрутизация (Routing)

Разработчик GUI может добавить секцию `routing` в конфиг клиента, чтобы клиент сам решал, что проксировать:
```json
"routing": {
  "default_policy": "proxy",
  "dns": {
    "strategy": "local",
    "local_servers": ["8.8.8.8"],
    "doh_url": "https://dns.google/dns-query"
  },
  "rules": [
    { "type": "domain",    "value": "mybank.ru",     "policy": "direct" },
    { "type": "domain",    "value": "ads.example.com", "policy": "block" },
    { "type": "geoip",     "value": "ru",             "policy": "direct" },
    { "type": "geosite",   "value": "category-ads",   "policy": "block" }
  ]
}
```

| Тип правила | Описание |
|-------------|----------|
| `domain` | Точный домен или домен с подстановкой |
| `geoip` | Страна по GeoIP базе (ISO-код) |
| `geosite` | Категория доменов из GeoSite базы |

| Политика | Описание |
|----------|----------|
| `proxy` | Направить через туннель |
| `direct` | Напрямую, минуя туннель |
| `block` | Заблокировать |

Для работы GeoIP/GeoSite требуются файлы `geoip.dat` и `geosite.dat` в директории `data/` (устанавливаются автоматически установщиком).

---
[Гайд по интеграции](integration.md) | [Назад в оглавление](README.md)
