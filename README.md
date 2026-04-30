# PP (Proxy Protocol)

> [!WARNING]
> **Отказ от ответственности:** Данный проект находится в стадии активной разработки и **не проходил аудит безопасности**. Используйте его на свой страх и риск. Протокол и формат конфигурации могут быть изменены без сохранения обратной совместимости.

**PP** — это модульная система проксирования трафика, ориентированная на маскировку соединений под обычный HTTPS-трафик (HTTP/2). Проект использует gRPC-туннелирование и шифрование Noise для обеспечения приватности.

## Основные возможности

*   **Маскировка (Fallback Facade)**: При обращении к серверу через обычный браузер отображается полноценный веб-сайт (блог), генерируемый «на лету» из RSS-лент или по ключевым словам.
*   **Транспорт**: Использование gRPC внутри HTTP/2 (через Nginx или напрямую), что делает трафик трудноотличимым от стандартного веб-серфинга.
*   **Шифрование**: Многослойная защита трафика с использованием Noise Protocol Framework и TLS.
*   **Централизованное управление**: Веб-интерфейс для настройки сервера, управления клиентами и автоматического получения SSL-сертификатов (Certbot).
*   **Full Tunnel**: Глобальное перенаправление TCP-трафика — через `iptables` на Linux, через маршрутизацию + System Proxy на Windows.
*   **Кроссплатформенность**: Полная поддержка Linux и Windows (amd64/arm64).

## Быстрый старт

### Серверная часть
Для установки на VPS (Ubuntu/Debian) выполните команду:
```bash
curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-server.sh | sudo bash
```
После установки веб-панель будет доступна по адресу `http://<IP-сервера>:4090`. Она поможет настроить домен и сгенерировать конфигурации для клиентов.

### Клиент — Linux
```bash
curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.sh | bash
```
```bash
pp-client start --config client.json
```

### Клиент — Windows
```powershell
irm https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.ps1 | iex
```
```powershell
pp-client start --config client.json --system-proxy
```

После запуска будут доступны локальные прокси: **SOCKS5** (`127.0.0.1:1080`) и **HTTP** (`127.0.0.1:8080`).

**Full Tunnel (Linux):**
```bash
sudo pp-client full-tunnel up --config client.json
```

**Full Tunnel (Windows — от Администратора):**
```powershell
pp-client full-tunnel up --config client.json
```

## Документация

*   **[Для администраторов](docs/admin/README.md)**: Подробные инструкции по установке, настройке маскировки и управлению подключениями.
*   **[Для разработчиков](docs/developer/README.md)**: Описание API клиента, формата конфигурации и гайд по интеграции `pp-client` в собственные GUI-приложения.
