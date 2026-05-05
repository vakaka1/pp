# pp-client как движок прокси

Бинарный файл `pp-client` (`pp-client.exe` на Windows) — это сердце клиентской части системы. Он берет на себя всю сложную работу: установку gRPC-соединения, Noise-шифрование, маскировку трафика и локальную маршрутизацию.

## Основная концепция

Для создания GUI-клиента вам не нужно реализовывать протоколы PP внутри своего кода. Ваше приложение (на Electron, Qt, Flutter или Go) выступает в роли «оболочки», которая:
1.  Формирует JSON-файл конфигурации.
2.  Запускает `pp-client` как дочерний процесс.
3.  Следит за состоянием туннеля через вывод логов в `stdout`.

## Установка

### Linux
```bash
curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.sh | bash
```

### Windows (PowerShell)
```powershell
irm https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-client.ps1 | iex
```

### Обновление Geo-баз
Отдельные скрипты для установки или обновления GeoIP/GeoSite баз:

**Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-geo.sh | bash
```

**Windows:**
```powershell
irm https://raw.githubusercontent.com/vakaka1/pp/main/scripts/install-geo.ps1 | iex
```

### Для GUI-приложения
GUI может поставлять `pp-client` в комплекте (bundled) или скачивать его при первом запуске из GitHub Releases:
```
https://github.com/vakaka1/pp/releases/latest/download/pp-client_linux_amd64
https://github.com/vakaka1/pp/releases/latest/download/pp-client_windows_amd64.zip
```

## Расположение файлов

### Linux

| Что | Путь (root) | Путь (пользователь) |
|-----|-------------|---------------------|
| Бинарник | `/usr/local/bin/pp-client` | `~/.local/bin/pp-client` |
| Конфиги | `/etc/pp-client/*.json` | `~/.config/pp-client/*.json` |
| GeoIP/GeoSite | `/var/lib/pp-client/data/` | `~/.local/share/pp-client/data/` |

### Windows

| Что | Путь |
|-----|------|
| Бинарник | `%LOCALAPPDATA%\pp\pp-client.exe` |
| Конфиги | `%APPDATA%\pp\*.json` |
| GeoIP/GeoSite | `%LOCALAPPDATA%\pp\data\` |

> Типичные значения переменных:
> - `%LOCALAPPDATA%` = `C:\Users\<User>\AppData\Local`
> - `%APPDATA%` = `C:\Users\<User>\AppData\Roaming`

### Порядок поиска конфигов

**Linux:** точный путь → `имя.json` в текущей директории → `configs/` → `/etc/pp/`

**Windows:** точный путь → `имя.json` в текущей директории → `%APPDATA%\pp\` → рядом с exe → `configs/`

## Полный справочник команд

### `pp-client version`
Выводит версию, дату сборки, коммит и платформу.
Пример:
```
PP-Client Version: v1.0.44
Build Date: 2026-04-30T09:42:06Z
Commit: 53e49e4
OS: windows/amd64
```

### `pp-client update`
Запускает официальный install-client скрипт для обновления текущего бинарника.
```bash
pp-client update
```
Поддерживается на Linux и Windows. На Linux команда использует `sudo`, если текущий процесс не root. На Windows стартует PowerShell-установщик.

### `pp-client start [имя_профиля]`
Основная команда — запуск прокси.
```bash
pp-client start --config client.json
pp-client start --config client.json --system-proxy
sudo pp-client start --config client.json --full-tunnel
pp-client start --config client.json --verbose
pp-client start client
```

| Флаг | Описание |
|------|----------|
| `--config` | Путь к JSON-конфигу или его имя |
| `--system-proxy` | Включить системный прокси (Windows: реестр, Linux: no-op) |
| `--full-tunnel` | Включить режим full-tunnel (маршрутизация всего трафика ОС). Требует root/admin. |
| `--owner` | UID или имя пользователя для исключения из full-tunnel (только Linux, по умолчанию `root` при `--full-tunnel`). |
| `--transparent-listen` | Адрес прозрачного слушателя (Linux full-tunnel) |
| `--verbose` | Включить DEBUG-логирование |

При запуске открываются локальные порты:
- **SOCKS5** — `127.0.0.1:1080` (по умолчанию)
- **HTTP** — `127.0.0.1:8080` (по умолчанию)
- **Transparent** — только на Linux при `--full-tunnel`; по умолчанию `127.0.0.1:1090`, если не задан `--transparent-listen` или `client.transparent_listen`.

`--full-tunnel` нужно передавать именно в `start`: клиент сначала включает системное перенаправление, затем запускает локальные прокси и при штатном завершении автоматически вызывает очистку full-tunnel. На Linux команда требует root, на Windows — запуск от Администратора.

### `pp-client test [имя_профиля]`
Проверяет конфиг и доступность сервера без запуска локальных прокси.
```bash
pp-client test --config client.json
pp-client test client
pp-client test --config client.json --verbose
```

Что делает команда:
- валидирует JSON-конфиг;
- выполняет browser-like проверку готовности сайта/маскировки сервера;
- пингует `client.server.domain`, а если он пустой — host из `client.server.address`;
- возвращает код `0` при успехе и `1` при ошибке.

У `test` нет `--full-tunnel`, `--system-proxy`, `--owner` и `--transparent-listen`: это диагностическая команда, она не меняет маршруты, iptables или системный прокси.

### `pp-client validate-config [имя_профиля]`
Проверка конфигурации без запуска.
```bash
pp-client validate-config --config client.json
pp-client validate-config client
```
Код возврата: `0` — валидный, `1` — ошибка.

### `pp-client import [uri]`
Импорт конфигурации из URI.
```bash
pp-client import "ppf://user@domain.com:443?proto=pp-fallback&pub=KEY&psk=KEY&path=/grpc"
```

| Платформа | Куда сохраняется |
|-----------|-----------------|
| Linux (root) | `/etc/pp/user.json` |
| Linux (user) | `configs/user.json` или `./user.json` |
| Windows | `%APPDATA%\pp\user.json` |

### `pp-client list`
Список всех импортированных профилей.
```bash
pp-client list
pp-client list --json
```
Флаг `--json` возвращает массив объектов — удобно для GUI:
```json
[
  {
    "name": "client",
    "path": "C:\\Users\\User\\AppData\\Roaming\\pp\\client.json",
    "meta": {
      "client_name": "client",
      "protocol": "pp-fallback",
      "generated_at": "2026-04-30T09:00:00Z"
    }
  }
]
```

### `pp-client delete [имя_профиля]`
Удаление профиля.
```bash
pp-client delete client
pp-client rm client
```

### Запуск по имени профиля
После импорта можно запускать по имени без указания полного пути:
```bash
pp-client start client
sudo pp-client start client --full-tunnel
```

### Full Tunnel

**Linux (требует root):**
```bash
sudo pp-client start --config client.json --full-tunnel
sudo pp-client start client --full-tunnel
sudo pp-client start --config client.json --full-tunnel --transparent-listen 127.0.0.1:12345 --owner root
```

**Windows (требует Администратора):**
```powershell
pp-client start --config client.json --full-tunnel
pp-client start client --full-tunnel
```

| Флаг | Описание |
|------|----------|
| `--config` | Путь к конфигу |
| `--transparent-listen` | Адрес прозрачного слушателя (только Linux) |
| `--owner` | UID/имя пользователя для исключения из перехвата (только Linux). Если не указан при `--full-tunnel`, используется `root`. |

На Linux `--full-tunnel` перенаправляет исходящий TCP через transparent listener. CLI автоматически исключает приватные/локальные сети, multicast/reserved-диапазоны, адрес PP-сервера и владельца процесса, указанного в `--owner`. Адрес сервера должен резолвиться в IPv4; IPv6 full-tunnel пока не поддерживается.

На Windows `--full-tunnel` добавляет split default routes (`0.0.0.0/1` и `128.0.0.0/1`) через текущий шлюз, добавляет отдельный маршрут до PP-сервера и включает System Proxy на адрес `client.http_proxy_listen` или `127.0.0.1:8080`.

При штатном завершении `start --full-tunnel` сам отключает full-tunnel. Команда `pp-client full-tunnel down` нужна как аварийная очистка после kill/crash или ручного вмешательства:
```bash
# Linux
sudo pp-client full-tunnel down

# Windows (Администратор)
pp-client full-tunnel down
```

## Как это работает внутри

### На обеих платформах
*   **Слушатели**: Клиент одновременно поднимает SOCKS5, HTTP и (на Linux) Прозрачный (Transparent) слушатели.
*   **Пул соединений**: Постоянное соединение с сервером минимизирует задержки при открытии новых сайтов.
*   **Маршрутизация**: Встроенный движок решает, какие сайты пускать через прокси, а какие — напрямую (GeoIP/GeoSite/domain rules).

### Linux Full Tunnel
Использует `iptables` для перенаправления исходящего TCP в прозрачный слушатель. Из перенаправления исключаются локальные/приватные сети, IP сервера и владелец процесса из `--owner`.

### Windows System Proxy
Записывает `ProxyServer` и `ProxyEnable=1` в реестр Windows (`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`), затем вызывает WinAPI `InternetSetOptionW` для немедленного применения. При завершении — сбрасывает обратно.

### Windows Full Tunnel
1. Определяет IP сервера PP (исключает из перенаправления).
2. Получает текущий шлюз через `Get-NetRoute`.
3. Добавляет маршруты `0.0.0.0/1` и `128.0.0.0/1` через реальный шлюз.
4. Включает System Proxy одновременно.
5. При штатном завершении `start --full-tunnel` удаляет маршруты и отключает прокси.

---
[Конфигурация клиента](config-client.md) | [Назад в оглавление](README.md)
