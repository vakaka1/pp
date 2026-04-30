#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Config,
    [string]$InstallDir,
    [switch]$Help
)

$ErrorActionPreference = "Stop"

$GITHUB_REPO = if ($env:GITHUB_REPO) { $env:GITHUB_REPO } else { "vakaka1/pp" }
$RELEASE_TAG = if ($env:RELEASE_TAG) { $env:RELEASE_TAG } else { "latest" }

$GEO_IP_URL  = "https://github.com/v2fly/geoip/releases/latest/download/geoip.dat"
$GEO_SITE_URL = "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"

function Write-Header {
    Write-Host ""
    Write-Host "  ╔══════════════════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "  ║            PP Client Installer (Windows)            ║" -ForegroundColor Cyan
    Write-Host "  ╚══════════════════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host "  GitHub:      github.com/$GITHUB_REPO" -ForegroundColor Gray
    Write-Host "  Arch:        windows/$script:GoArch" -ForegroundColor Gray
    Write-Host ""
}

function Write-Step  { param([string]$Msg) Write-Host "`n  ▶ $Msg" -ForegroundColor Blue }
function Write-Ok    { param([string]$Msg) Write-Host "  ✔ $Msg" -ForegroundColor Green }
function Write-Info  { param([string]$Msg) Write-Host "  ℹ $Msg" -ForegroundColor Cyan }
function Write-Warn  { param([string]$Msg) Write-Host "  ⚠ $Msg" -ForegroundColor Yellow }
function Write-Err   { param([string]$Msg) Write-Host "  ✖ $Msg" -ForegroundColor Red }

function Exit-Fatal {
    param([string]$Msg)
    Write-Err $Msg
    exit 1
}

function Show-Usage {
    Write-Host @"

  Использование:
    irm https://raw.githubusercontent.com/$GITHUB_REPO/main/scripts/install-client.ps1 | iex

  Или скачать и запустить:
    .\install-client.ps1 [-InstallDir C:\path] [-Config client.json]

  Параметры:
    -InstallDir   Директория установки (по умолчанию %LOCALAPPDATA%\pp)
    -Config       Путь к файлу конфигурации для активации после установки
    -Help         Показать эту справку

  Что делает:
    - Скачивает pp-client.exe из GitHub Releases
    - Устанавливает в %LOCALAPPDATA%\pp\
    - Скачивает GeoIP / GeoSite базы
    - Добавляет директорию в PATH (для текущего пользователя)
    - Создаёт лаунчер-скрипты (pp-start.cmd, pp-tunnel.cmd)
"@
}

function Get-GoArch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64"  { return "amd64" }
        "ARM64"  { return "arm64" }
        default  { Exit-Fatal "Архитектура $arch не поддерживается" }
    }
}

function Get-LatestVersion {
    try {
        $headers = @{}
        if ($env:GITHUB_TOKEN) {
            $headers["Authorization"] = "token $env:GITHUB_TOKEN"
        }
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$GITHUB_REPO/releases/latest" -Headers $headers -TimeoutSec 30
        return $release.tag_name
    } catch {
        Exit-Fatal "Не удалось получить информацию о последнем релизе: $_"
    }
}

function Download-File {
    param(
        [string]$Url,
        [string]$Destination,
        [string]$Description
    )
    Write-Info "Загрузка: $Description..."
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $Url -OutFile $Destination -UseBasicParsing -TimeoutSec 120
        $ProgressPreference = 'Continue'
        $size = (Get-Item $Destination).Length
        $sizeMB = [math]::Round($size / 1MB, 1)
        Write-Ok "$Description ($sizeMB MB)"
    } catch {
        Exit-Fatal "Не удалось скачать $Description : $_"
    }
}

function Add-ToUserPath {
    param([string]$Dir)

    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -and $currentPath.Split(";") -contains $Dir) {
        Write-Info "Директория уже в PATH"
        return $false
    }

    $newPath = if ($currentPath) { "$currentPath;$Dir" } else { $Dir }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")

    $env:Path = "$env:Path;$Dir"

    Write-Ok "Добавлено в PATH пользователя: $Dir"
    return $true
}

function Install-PPClient {
    $script:GoArch = Get-GoArch

    if ($Help) {
        Show-Usage
        return
    }

    if (-not $InstallDir) {
        $InstallDir = Join-Path $env:LOCALAPPDATA "pp"
    }

    $binDir  = $InstallDir
    $dataDir = Join-Path $InstallDir "data"
    $cfgDir  = Join-Path $env:APPDATA "pp"
    $exePath = Join-Path $binDir "pp-client.exe"

    Write-Header

    Write-Step "Определение версии"
    if ($RELEASE_TAG -eq "latest") {
        $version = Get-LatestVersion
    } else {
        $version = $RELEASE_TAG
    }
    Write-Ok "Версия: $version"

    Write-Step "Подготовка директорий"
    foreach ($dir in @($binDir, $dataDir, $cfgDir)) {
        if (-not (Test-Path $dir)) {
            New-Item -ItemType Directory -Path $dir -Force | Out-Null
        }
    }
    Write-Ok "Директории созданы"

    Write-Step "Загрузка компонентов"

    $downloadUrl = "https://github.com/$GITHUB_REPO/releases/download/$version/pp-client_windows_$($script:GoArch).zip"
    $zipPath = Join-Path $env:TEMP "pp-client-$version.zip"
    $extractDir = Join-Path $env:TEMP "pp-client-extract"

    Download-File -Url $downloadUrl -Destination $zipPath -Description "pp-client.exe"

    Write-Info "Распаковка архива..."
    if (Test-Path $extractDir) {
        Remove-Item -Recurse -Force $extractDir
    }
    Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

    $extractedExe = Get-ChildItem -Path $extractDir -Filter "pp-client.exe" -Recurse | Select-Object -First 1
    if (-not $extractedExe) {
        Exit-Fatal "pp-client.exe не найден в архиве"
    }
    Copy-Item -Path $extractedExe.FullName -Destination $exePath -Force
    Remove-Item -Recurse -Force $extractDir -ErrorAction SilentlyContinue
    Remove-Item -Force $zipPath -ErrorAction SilentlyContinue

    Write-Ok "pp-client.exe установлен"

    try {
        $verOutput = & $exePath version 2>&1 | Out-String
        Write-Info $verOutput.Trim()
    } catch {}

    $geoIpFile = Join-Path $dataDir "geoip.dat"
    $geoSiteFile = Join-Path $dataDir "geosite.dat"

    $needGeoIp = (-not (Test-Path $geoIpFile)) -or ((Get-Item $geoIpFile).Length -lt 1000000)
    $needGeoSite = (-not (Test-Path $geoSiteFile)) -or ((Get-Item $geoSiteFile).Length -lt 100000)

    if ($needGeoIp) {
        Download-File -Url $GEO_IP_URL -Destination $geoIpFile -Description "GeoIP база"
    } else {
        Write-Ok "GeoIP база актуальна"
    }

    if ($needGeoSite) {
        Download-File -Url $GEO_SITE_URL -Destination $geoSiteFile -Description "GeoSite база"
    } else {
        Write-Ok "GeoSite база актуальна"
    }

    Write-Step "Настройка скриптов запуска"

    $startScript = Join-Path $binDir "pp-start.cmd"
    $tunnelScript = Join-Path $binDir "pp-tunnel.cmd"

    $startContent = @"
@echo off
setlocal
set PP_BIN=$exePath
set PP_DATA=$dataDir
set PP_CONFIG=$cfgDir\client.json

if not exist "%PP_BIN%" (
    echo pp-client.exe not found: %PP_BIN% 1>&2
    exit /b 1
)
if not exist "%PP_CONFIG%" (
    echo Config not found: %PP_CONFIG% 1>&2
    echo Run: pp-client import "ppf://..." 1>&2
    exit /b 1
)

cd /d "%PP_DATA%\.."
"%PP_BIN%" start --config "%PP_CONFIG%" --system-proxy %*
"@

    $tunnelContent = @"
@echo off
setlocal
set PP_BIN=$exePath
set PP_DATA=$dataDir
set PP_CONFIG=$cfgDir\client.json

net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Full-tunnel requires Administrator privileges. 1>&2
    echo Right-click and select "Run as administrator". 1>&2
    exit /b 1
)

if not exist "%PP_BIN%" (
    echo pp-client.exe not found: %PP_BIN% 1>&2
    exit /b 1
)
if not exist "%PP_CONFIG%" (
    echo Config not found: %PP_CONFIG% 1>&2
    echo Run: pp-client import "ppf://..." 1>&2
    exit /b 1
)

cd /d "%PP_DATA%\.."
"%PP_BIN%" start --config "%PP_CONFIG%" --system-proxy &
timeout /t 3 >nul
"%PP_BIN%" full-tunnel up --config "%PP_CONFIG%"
echo.
echo Full-tunnel active. Press Ctrl+C to stop.
echo.

:wait
timeout /t 86400 >nul
goto wait
"@

    Set-Content -Path $startScript -Value $startContent -Encoding ASCII
    Set-Content -Path $tunnelScript -Value $tunnelContent -Encoding ASCII
    Write-Ok "Созданы: pp-start.cmd, pp-tunnel.cmd"

    Write-Step "Настройка PATH"
    $pathAdded = Add-ToUserPath -Dir $binDir

    if ($Config -and (Test-Path $Config)) {
        Write-Step "Активация конфигурации"
        $destConfig = Join-Path $cfgDir "client.json"
        Copy-Item -Path $Config -Destination $destConfig -Force
        Write-Ok "Конфиг скопирован в $destConfig"
    }

    Write-Host ""
    Write-Host "  ╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "  ║              Установка клиента завершена!                   ║" -ForegroundColor Green
    Write-Host "  ╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Пути:" -ForegroundColor White
    Write-Host "    Бинарник:    $exePath" -ForegroundColor Cyan
    Write-Host "    Данные:      $dataDir" -ForegroundColor Cyan
    Write-Host "    Конфиги:     $cfgDir" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  Использование:" -ForegroundColor White
    Write-Host "    Импорт:           pp-client import `"ppf://...`"" -ForegroundColor Cyan
    Write-Host "    Обычный режим:    pp-start" -ForegroundColor Cyan
    Write-Host "    Или напрямую:     pp-client start --config client.json --system-proxy" -ForegroundColor Cyan
    Write-Host "    Full-tunnel:      pp-tunnel  (от Администратора)" -ForegroundColor Cyan
    Write-Host ""

    if ($pathAdded) {
        Write-Warn "PATH обновлён. Перезапустите терминал, чтобы изменения вступили в силу."
    }
}

Install-PPClient
