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

function Write-Info  { param([string]$Msg) Write-Host -NoNewline -ForegroundColor Cyan "ℹ " ; Write-Host $Msg }
function Write-Ok    { param([string]$Msg) Write-Host -NoNewline -ForegroundColor Green "✔ " ; Write-Host $Msg }
function Write-Warn  { param([string]$Msg) Write-Host -NoNewline -ForegroundColor Yellow "⚠ " ; Write-Host $Msg }
function Write-Err   { param([string]$Msg) Write-Host -NoNewline -ForegroundColor Red "✖ " ; Write-Host $Msg }
function Write-Step  { param([string]$Msg) Write-Host "`n" ; Write-Host -NoNewline -ForegroundColor Blue "▶ " ; Write-Host $Msg -ForegroundColor White }

function Exit-Fatal {
    param([string]$Msg)
    Write-Err $Msg
    exit 1
}

function Show-Usage {
    Write-Host @"
Использование: install-client.ps1

Скрипт устанавливает клиентские инструменты PP (Windows):
- pp-client.exe (основной бинарник)

Параметры:
  -InstallDir   Директория установки (по умолчанию %LOCALAPPDATA%\pp)
  -Config       Путь к файлу конфигурации для активации после установки
  -Help         Показать эту справку
"@
    exit 0
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
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $Url -OutFile $Destination -UseBasicParsing -TimeoutSec 120
        $ProgressPreference = 'Continue'
        Write-Ok $Description
    } catch {
        Exit-Fatal "Не удалось скачать $Description : $_"
    }
}

function Add-ToUserPath {
    param([string]$Dir)

    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($currentPath -and $currentPath.Split(";") -contains $Dir) {
        return $false
    }

    $newPath = if ($currentPath) { "$currentPath;$Dir" } else { $Dir }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")

    $env:Path = "$env:Path;$Dir"
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

    Clear-Host
    Write-Host ""
    Write-Host "╔══════════════════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "║                 PP Client Installer                  ║" -ForegroundColor Cyan
    Write-Host "╚══════════════════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host -NoNewline "  GitHub:      " ; Write-Host "github.com/$GITHUB_REPO" -ForegroundColor Cyan
    Write-Host -NoNewline "  Архитектура: " ; Write-Host "windows/$script:GoArch" -ForegroundColor Cyan
    Write-Host ""

    Write-Info "Получение информации о релизе..."
    if ($RELEASE_TAG -eq "latest") {
        $version = Get-LatestVersion
    } else {
        $version = $RELEASE_TAG
    }
    Write-Ok "Версия для установки: $version"

    Write-Step "Подготовка файловой системы"
    foreach ($dir in @($binDir, $dataDir, $cfgDir)) {
        if (-not (Test-Path $dir)) {
            New-Item -ItemType Directory -Path $dir -Force | Out-Null
        }
    }
    Write-Ok "Создание директорий ($InstallDir)"

    Write-Step "Загрузка компонентов"

    $downloadUrl = "https://github.com/$GITHUB_REPO/releases/download/$version/pp-client_windows_$($script:GoArch).zip"
    $zipPath = Join-Path $env:TEMP "pp-client-$version.zip"
    $extractDir = Join-Path $env:TEMP "pp-client-extract"

    Download-File -Url $downloadUrl -Destination $zipPath -Description "Загрузка бинарника pp"

    if (Test-Path $extractDir) {
        Remove-Item -Recurse -Force $extractDir
    }
    Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

    $extractedExe = Get-ChildItem -Path $extractDir -Filter "pp-client*.exe" -Recurse | Select-Object -First 1
    if (-not $extractedExe) {
        Exit-Fatal "pp-client*.exe не найден в архиве"
    }
    
    Copy-Item -Path $extractedExe.FullName -Destination $exePath -Force
    Remove-Item -Recurse -Force $extractDir -ErrorAction SilentlyContinue
    Remove-Item -Force $zipPath -ErrorAction SilentlyContinue

    try {
        $verOutput = & $exePath version 2>&1 | Out-String
        Write-Ok "Установлена версия: $($verOutput.Trim())"
    } catch {}

    
    $pathAdded = Add-ToUserPath -Dir $binDir

    if ($Config -and (Test-Path $Config)) {
        Write-Step "Активация конфигурации"
        $destConfig = Join-Path $cfgDir "client.json"
        Copy-Item -Path $Config -Destination $destConfig -Force
        Write-Ok "Конфиг скопирован в $destConfig"
    }

    Write-Host ""
    Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "║                Установка клиента завершена!                  ║" -ForegroundColor Green
    Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Пути:" -ForegroundColor White
    Write-Host -NoNewline "    Бинарник:    " ; Write-Host $exePath -ForegroundColor Cyan
    Write-Host -NoNewline "    Данные:      " ; Write-Host $dataDir -ForegroundColor Cyan
    Write-Host -NoNewline "    Конфиги:     " ; Write-Host $cfgDir -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  Использование:" -ForegroundColor White
    Write-Host -NoNewline "    Подключение:     " ; Write-Host "pp-client start --config client.json" -ForegroundColor Cyan
    Write-Host -NoNewline "    Full-tunnel:     " ; Write-Host "pp-client full-tunnel up --config client.json (от Администратора)" -ForegroundColor Cyan
    Write-Host ""

    if ($pathAdded) {
        Write-Warn "Каталог $binDir был добавлен в PATH. Перезапустите терминал для применения."
    }
}

Install-PPClient
