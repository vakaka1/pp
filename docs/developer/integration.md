# Гайд по интеграции с GUI

Этот раздел содержит практические рекомендации по встраиванию `pp-client` в ваше GUI-приложение на любой платформе.

## Установка и обновление pp-client

### Автоматическая установка (рекомендуется для ручной установки)

**Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.sh | bash
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.ps1 | iex
```

### Встраивание в GUI-приложение

GUI может либо поставлять `pp-client` в комплекте, либо скачивать из GitHub Releases при первом запуске:

```
# Linux
https://github.com/vakaka1/pp/releases/latest/download/pp-client_linux_amd64

# Windows (ZIP-архив)
https://github.com/vakaka1/pp/releases/latest/download/pp-client_windows_amd64.zip
```

Рекомендуемые пути для размещения бинарника из GUI:

| Платформа | Путь |
|-----------|------|
| Linux | `~/.local/bin/pp-client` или рядом с GUI |
| Windows | `%LOCALAPPDATA%\pp\pp-client.exe` или рядом с GUI exe |

## Управление процессом

Запускайте `pp-client` как дочерний процесс вашего GUI.

### Запуск

**Flutter/Dart:**
```dart
// Linux
final process = await Process.start('/home/user/.local/bin/pp-client', [
  'start', '--config', configPath,
]);

// Windows
final process = await Process.start(r'C:\Users\User\AppData\Local\pp\pp-client.exe', [
  'start', '--config', configPath, '--system-proxy',
]);
```

**Go:**
```go
cmd := exec.CommandContext(ctx, ppClientPath, "start", "--config", configPath, "--system-proxy")
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
err := cmd.Start()
```

### Завершение

| Платформа | Способ | Код |
|-----------|--------|-----|
| Linux | `SIGTERM` / `SIGINT` | `process.kill(ProcessSignal.sigterm)` |
| Windows | `taskkill` / terminate | `process.kill()` или `taskkill /PID <pid> /F` |

При корректном завершении клиент автоматически:
- Закрывает все соединения
- Отключает System Proxy (если был `--system-proxy`)
- НЕ откатывает full-tunnel маршруты (нужно отдельно вызвать `full-tunnel down`)

### Аварийное завершение

Если процесс был убит без корректного завершения:
- **Windows:** System Proxy может остаться включённым в реестре. GUI должен вызвать:
  ```
  pp-client.exe full-tunnel down
  ```
  Или напрямую сбросить реестр:
  ```powershell
  Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -Name ProxyEnable -Value 0
  ```
- **Linux:** Правила iptables могут остаться. GUI должен вызвать:
  ```
  sudo pp-client full-tunnel down
  ```

## Режимы работы

### Обычный режим

Подходит для повседневного использования. Приложения, которые используют системные настройки прокси (браузеры, большинство программ), будут работать через туннель.

**Linux:**
```bash
pp-client start --config client.json
```
GUI должен настроить прокси вручную (переменные окружения `http_proxy`, `https_proxy` или настройки рабочего стола).

**Windows:**
```powershell
pp-client.exe start --config client.json --system-proxy
```
Флаг `--system-proxy` автоматически включает прокси в реестре Windows и вызывает `InternetSetOptionW` для немедленного применения. При завершении процесса прокси отключается.

### Full Tunnel

Весь TCP-трафик системы перенаправляется через туннель. Требует повышенных привилегий.

**Linux (требует root):**
```bash
# Шаг 1: Запуск клиента с прозрачным слушателем
pp-client start --config client.json --transparent-listen 127.0.0.1:12345

# Шаг 2: Включение iptables-перенаправления (в отдельном процессе)
sudo pp-client full-tunnel up --config client.json --transparent-listen 127.0.0.1:12345 --owner $(whoami)
```
Для GUI используйте `pkexec` для шага 2:
```bash
pkexec pp-client full-tunnel up --config client.json --transparent-listen 127.0.0.1:12345 --owner $USER
```

**Windows (требует Администратора):**
```powershell
# Шаг 1: Запуск клиента
pp-client.exe start --config client.json

# Шаг 2: Включение маршрутов (в отдельном процессе от Администратора)
pp-client.exe full-tunnel up --config client.json
```
Для GUI: запустите процесс с `runas` или проверяйте права через `net session`:
```dart
// Dart/Flutter — запуск с повышенными привилегиями
final process = await Process.start('powershell', [
  '-Command', 'Start-Process', 'pp-client.exe',
  '-ArgumentList', '"full-tunnel up --config client.json"',
  '-Verb', 'RunAs',
]);
```

**Отключение Full Tunnel:**
```bash
# Linux
sudo pp-client full-tunnel down

# Windows (Администратор)
pp-client.exe full-tunnel down
```

## Импорт конфигурации (ppf:// URI)

URI формата `ppf://` позволяет импортировать настройки одной командой:
```bash
pp-client import "ppf://client-name@server:443?proto=pp-fallback&pub=KEY&psk=PSK&path=/grpc"
```

GUI может вызвать эту команду программно — она создаст JSON-конфиг автоматически.

| Платформа | Куда сохраняется |
|-----------|-----------------|
| Linux (root) | `/etc/pp/client-name.json` |
| Linux (user) | `configs/client-name.json` или `./client-name.json` |
| Windows | `%APPDATA%\pp\client-name.json` |

## Получение списка профилей

```bash
pp-client list --json
```

Возвращает JSON-массив — используйте для отрисовки списка серверов в GUI:
```json
[
  {
    "name": "work-server",
    "path": "C:\\Users\\User\\AppData\\Roaming\\pp\\work-server.json",
    "meta": {
      "client_name": "work-server",
      "protocol": "pp-fallback",
      "generated_at": "2026-04-30T09:00:00Z"
    }
  }
]
```

## Мониторинг состояния (Логи)

Клиент пишет логи в `stdout`. Парсите вывод для определения состояния:

| Сообщение | Значение |
|-----------|----------|
| `SOCKS5 server started {"address": "127.0.0.1:1080"}` | Движок готов, прокси работает |
| `HTTP proxy server started {"address": "127.0.0.1:8080"}` | HTTP прокси работает |
| `system proxy enabled {"address": "127.0.0.1:8080"}` | Windows System Proxy активирован |
| `system proxy disabled` | Windows System Proxy отключён |
| `transparent proxy server started` | Прозрачный слушатель для full-tunnel запущен |
| `failed to open stream` | Сервер недоступен или ключи неверны |
| `client error` | Критическая ошибка, процесс завершается |

## Валидация конфига

Перед запуском проверяйте конфиг:
```bash
pp-client validate-config --config client.json
```
- Код возврата `0` — конфиг валиден.
- Код возврата `1` — ошибка (описание в stdout).

## Расположение файлов — сводная таблица

| Компонент | Linux (root) | Linux (user) | Windows |
|-----------|-------------|-------------|---------|
| Бинарник | `/usr/local/bin/pp-client` | `~/.local/bin/pp-client` | `%LOCALAPPDATA%\pp\pp-client.exe` |
| Конфиги | `/etc/pp-client/*.json` | `~/.config/pp-client/*.json` | `%APPDATA%\pp\*.json` |
| GeoIP | `/var/lib/pp-client/data/geoip.dat` | `~/.local/share/pp-client/data/geoip.dat` | `%LOCALAPPDATA%\pp\data\geoip.dat` |
| GeoSite | `/var/lib/pp-client/data/geosite.dat` | `~/.local/share/pp-client/data/geosite.dat` | `%LOCALAPPDATA%\pp\data\geosite.dat` |
| PATH | `/usr/local/bin` или `~/.local/bin` | — | `%LOCALAPPDATA%\pp` (добавляется установщиком) |


## Совет по безопасности

Не храните PSK и приватные ключи в открытом виде в логах GUI. Хотя `pp-client` скрывает чувствительные данные в обычном режиме, при `--verbose` некоторые параметры могут попасть в вывод.

---
[Назад в оглавление](README.md)
