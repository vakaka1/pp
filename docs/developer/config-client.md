# Конфигурация клиента

Конфигурационный файл клиента — это JSON, который содержит параметры сервера и настройки локальных сервисов.

## Пример структуры

```json
{
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
      "grpc_path": "/pp.v1.TunnelService/Connect"
    },
    "transport": {
      "shaper_enabled": true,
      "keepalive_interval_seconds": 25
    }
  }
}
```

## Описание параметров

### Секция `client`
*   **socks5_listen**: Адрес, на котором будет поднят SOCKS5 прокси.
*   **http_proxy_listen**: Адрес для HTTP прокси.
*   **transparent_listen**: (Опционально) Порт для прозрачного проксирования (используется для Full Tunnel).

### Секция `client.server`
*   **address**: IP или Домен сервера с портом (обычно 443).
*   **domain**: Домен для проверки SSL-сертификата.
*   **noise_public_key**: Публичный ключ сервера (Noise). Генерируется на сервере.
*   **psk**: Личный ключ доступа клиента.
*   **grpc_path**: Секретный путь к gRPC-сервису. Должен совпадать с настройками сервера.
*   **tls_fingerprint**: (Опционально) Имитация отпечатка браузера (например, `chrome`).

### Секция `client.transport`
*   **shaper_enabled**: Включает сглаживание трафика для защиты от анализа по паттернам.
*   **jitter_max_ms**: Максимальная задержка для имитации естественного поведения.
*   **reconnect_stream_min / max**: Настройки автоматического пересоздания стримов внутри туннеля для повышения скрытности.

## Маршрутизация (Routing)
Разработчик GUI может добавить секцию `routing`, чтобы клиент сам решал, что проксировать:
```json
"routing": {
  "default_policy": "proxy",
  "rules": [
    { "type": "domain", "value": "mybank.ru", "policy": "direct" },
    { "type": "geoip", "value": "ru", "policy": "direct" }
  ]
}
```

---
[Гайд по интеграции](integration.md) | [Назад в оглавление](README.md)
