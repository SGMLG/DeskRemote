# DeskRemote 1-Click Local Build Script
$ErrorActionPreference = "Stop"

$DistDir = Join-Path $PSScriptRoot "dist"
if (Test-Path $DistDir) { Remove-Item -Recurse -Force $DistDir }
New-Item -ItemType Directory -Path $DistDir -Force | Out-Null

Write-Host ">>> [1/4] Downloading cloudflared.exe..." -ForegroundColor Cyan
Invoke-WebRequest -Uri "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe" -OutFile "$DistDir\cloudflared.exe"

Write-Host ">>> [2/4] Downloading ffmpeg.exe..." -ForegroundColor Cyan
$ffmpegZip = "$env:TEMP\ffmpeg_temp.zip"
Invoke-WebRequest -Uri "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip" -OutFile $ffmpegZip
Expand-Archive -Path $ffmpegZip -DestinationPath "$env:TEMP\ffmpeg_extracted" -Force
$ffmpegExe = (Get-ChildItem -Path "$env:TEMP\ffmpeg_extracted" -Filter "ffmpeg.exe" -Recurse).FullName
Copy-Item $ffmpegExe -Destination "$DistDir\ffmpeg.exe" -Force
Remove-Item $ffmpegZip, "$env:TEMP\ffmpeg_extracted" -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ">>> [3/4] Compiling DeskRemote.exe..." -ForegroundColor Cyan
go build -ldflags "-H=windowsgui -s -w" -o "$DistDir\DeskRemote.exe" .

Write-Host ">>> [4/4] Creating Release ZIP..." -ForegroundColor Cyan
$version = Get-Content (Join-Path $PSScriptRoot "VERSION") -ErrorAction SilentlyContinue
if (-not $version) { $version = "1.0.0" }
$zipName = "DeskRemote-v$version-Windows-x64.zip"
Compress-Archive -Path "$DistDir\*" -DestinationPath (Join-Path $PSScriptRoot $zipName) -Force

Write-Host ">>> Build successful! Created $zipName" -ForegroundColor Green
