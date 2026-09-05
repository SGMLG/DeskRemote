# DeskRemote 1-Click Local Build Script
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$DistDir = Join-Path $PSScriptRoot "dist"
if (-not (Test-Path $DistDir)) {
    New-Item -ItemType Directory -Path $DistDir -Force | Out-Null
}

# 1. cloudflared.exe
$targetCloudflared = Join-Path $DistDir "cloudflared.exe"
$localCloudflared = Join-Path $PSScriptRoot "cloudflared.exe"

if (Test-Path $localCloudflared) {
    Write-Host ">>> [1/4] Using local cloudflared.exe..." -ForegroundColor Green
    Copy-Item $localCloudflared -Destination $targetCloudflared -Force
} elseif (-not (Test-Path $targetCloudflared)) {
    Write-Host ">>> [1/4] Downloading cloudflared.exe..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe" -OutFile $targetCloudflared
} else {
    Write-Host ">>> [1/4] Existing cloudflared.exe found in dist." -ForegroundColor Green
}

# 2. ffmpeg.exe
$targetFfmpeg = Join-Path $DistDir "ffmpeg.exe"
$localFfmpeg = Join-Path $PSScriptRoot "ffmpeg.exe"

if (Test-Path $localFfmpeg) {
    Write-Host ">>> [2/4] Using local ffmpeg.exe..." -ForegroundColor Green
    Copy-Item $localFfmpeg -Destination $targetFfmpeg -Force
} elseif (-not (Test-Path $targetFfmpeg)) {
    Write-Host ">>> [2/4] Downloading ffmpeg.exe..." -ForegroundColor Cyan
    $ffmpegZip = "$env:TEMP\ffmpeg_temp.zip"
    Invoke-WebRequest -Uri "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip" -OutFile $ffmpegZip
    Expand-Archive -Path $ffmpegZip -DestinationPath "$env:TEMP\ffmpeg_extracted" -Force
    $ffmpegExe = (Get-ChildItem -Path "$env:TEMP\ffmpeg_extracted" -Filter "ffmpeg.exe" -Recurse).FullName
    Copy-Item $ffmpegExe -Destination $targetFfmpeg -Force
    Remove-Item $ffmpegZip, "$env:TEMP\ffmpeg_extracted" -Recurse -Force -ErrorAction SilentlyContinue
} else {
    Write-Host ">>> [2/4] Existing ffmpeg.exe found in dist." -ForegroundColor Green
}

# 3. Compile DeskRemote.exe
Write-Host ">>> [3/4] Compiling DeskRemote.exe..." -ForegroundColor Cyan
go build -ldflags "-H=windowsgui -s -w" -o "$DistDir\DeskRemote.exe" .

# 4. Release ZIP
Write-Host ">>> [4/4] Creating Release ZIP..." -ForegroundColor Cyan
$version = Get-Content (Join-Path $PSScriptRoot "VERSION") -ErrorAction SilentlyContinue
if (-not $version) { $version = "1.3.0" }
$zipName = "DeskRemote-v$version-Windows-x64.zip"
$zipPath = Join-Path $PSScriptRoot $zipName
if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Compress-Archive -Path "$DistDir\*" -DestinationPath $zipPath -Force

Write-Host ">>> Build successful! Created $zipName" -ForegroundColor Green
