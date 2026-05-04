#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$InstallDir
)

$ErrorActionPreference = "Stop"

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

if (-not $InstallDir) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "pp"
}
$DataDir = Join-Path $InstallDir "data"

if (-not (Test-Path $DataDir)) {
    New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
}

$GEO_IP_URL = "https://github.com/v2fly/geoip/releases/latest/download/geoip.dat"
$GEO_SITE_URL = "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"
$GEO_IP_SUM_URL = "$GEO_IP_URL.sha256sum"
$GEO_SITE_SUM_URL = "$GEO_SITE_URL.sha256sum"

$geoipFile = Join-Path $DataDir "geoip.dat"
$geositeFile = Join-Path $DataDir "geosite.dat"

function Get-RemoteSha256($url) {
    try {
        $ProgressPreference = 'SilentlyContinue'
        $resp = Invoke-RestMethod -Uri $url -UseBasicParsing -TimeoutSec 30
        $ProgressPreference = 'Continue'
        return ($resp -split '\s+')[0]
    } catch {
        return ""
    }
}

function Get-LocalSha256($path) {
    if (Test-Path $path) {
        $hash = Get-FileHash -Path $path -Algorithm SHA256
        return $hash.Hash.ToLower()
    }
    return ""
}

function Download-GeoFile {
    param(
        [string]$Url,
        [string]$Destination,
        [string]$Description
    )
    try {
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $Url -OutFile "$Destination.tmp" -UseBasicParsing -TimeoutSec 120
        $ProgressPreference = 'Continue'
        Move-Item -Path "$Destination.tmp" -Destination $Destination -Force
        Write-Ok $Description
    } catch {
        Exit-Fatal "Не удалось скачать базу: $_"
    }
}

Clear-Host
Write-Host ""
Write-Host "╔══════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║               PP Geo-Database Installer              ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""
Write-Info "Целевая директория: $DataDir"

Write-Step "Проверка GeoIP базы..."
$remoteIpSum = Get-RemoteSha256 $GEO_IP_SUM_URL
$localIpSum = Get-LocalSha256 $geoipFile

$updateIp = $true
if ($remoteIpSum -ne "" -and $remoteIpSum -eq $localIpSum) {
    Write-Ok "GeoIP база уже актуальна."
    $updateIp = $false
} else {
    Write-Info "Требуется обновление GeoIP базы."
}

Write-Step "Проверка GeoSite базы..."
$remoteSiteSum = Get-RemoteSha256 $GEO_SITE_SUM_URL
$localSiteSum = Get-LocalSha256 $geositeFile

$updateSite = $true
if ($remoteSiteSum -ne "" -and $remoteSiteSum -eq $localSiteSum) {
    Write-Ok "GeoSite база уже актуальна."
    $updateSite = $false
} else {
    Write-Info "Требуется обновление GeoSite базы."
}

if ($updateIp) {
    Write-Step "Загрузка баз"
    Download-GeoFile -Url $GEO_IP_URL -Destination $geoipFile -Description "Загрузка GeoIP базы"
}

if ($updateSite) {
    if (-not $updateIp) { Write-Step "Загрузка баз" }
    Download-GeoFile -Url $GEO_SITE_URL -Destination $geositeFile -Description "Загрузка GeoSite базы"
}

Write-Host ""
Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "║                Установка баз завершена!                      ║" -ForegroundColor Green
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Green
Write-Host ""
