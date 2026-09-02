# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.1] - 2026-09-02

### Changed
- Disabled distracting Windows popup/balloon notification on startup when link is ready (link is still copied to clipboard silently, and tray tooltip updates to "DeskRemote: Активен").

## [1.0.0] - 2026-09-02

### Added
- **WebRTC Desktop Streaming**: Low-latency 60 FPS real-time screen streaming via VP8 (libvpx) & Pion WebRTC.
- **Zero-Config Cloudflare Tunnel**: Automatic HTTPS/WSS proxying with trycloudflare quick tunnels (bypasses NAT, firewalls, and restricted university/office networks without port forwarding).
- **Web Client UI**:
  - Auto-fullscreen mode on PIN submission.
  - Dedicated "⛶ На весь экран" toggle.
  - Full mobile & tablet touch support (taps, drags, multi-touch prevention).
  - PIN code access security modal.
- **Windows System Tray & Autostart**:
  - Background system tray icon with context menu.
  - 1-click clipboard copy of connection URL.
  - "Open in browser" / double-click launcher.
  - Autostart toggle via Windows registry (`HKCU\...\Run`).
- **CI/CD & Automation**:
  - GitHub Actions automated release pipeline.
  - One-click build script (`build.ps1`).
