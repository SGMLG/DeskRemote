# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.4.0] - 2026-09-04

### Added
- **Hardware Video Acceleration & 60 FPS**:
  - Automatic GPU encoder detection with dynamic probing: priority for AMD AMF (`h264_amf`), NVIDIA NVENC (`h264_nvenc`), Intel QuickSync (`h264_qsv`), with automatic fallback to optimized software `libx264`.
  - Upgraded video stream to **60 FPS** (16.6ms frame pacing) with zero-latency tuning (`-bf 0`, `-tune zerolatency` / `-usage ultralowlatency`).
  - Standardized on H.264 Constrained Baseline Profile (Level 3.1, `profile-level-id=42e01f`) for 100% native mobile hardware decoding without battery drain.
- **System Audio Streaming (DirectShow -> WebRTC Opus)**:
  - Added dedicated `AudioBroadcaster` singleton capturing Windows audio via DirectShow and encoding to low-delay Opus stereo at 64 kbps.
  - Multi-track WebRTC negotiation (`pc.addTransceiver('audio', ...)`) binding both video and audio into a single synchronized `MediaStream`.
  - Added interactive Audio Toggle button (🔊 / 🔇) in the floating dock with mobile autoplay compliance and state persistence.
- **Progressive Web App (PWA) & Screen WakeLock**:
  - Native PWA manifest (`/manifest.json`) and vector app icon (`/icon.svg`) for installing DeskRemote directly to home screen (standalone mode without browser URL bars).
  - Integrated Screen WakeLock API (`navigator.wakeLock.request('screen')`) to prevent mobile screens from sleeping or dimming while remotely controlling the PC, with auto-recovery on tab visibility change.

## [1.3.0] - 2026-09-03

### Added
- **Hybrid HUD & YouTube-Style Dock**:
  - Replaced legacy static layouts with a cinematic Hybrid HUD featuring a floating frosted-glass bottom toolbar.
  - Live WebRTC connection indicator (`● LIVE`), real-time session stopwatch timer, and latency tag (`12 ms`).
  - Red HD 1080p stream badge, fullscreen toggle, and instant PIN reset/disconnect button.
- **Cinematic Genie Effect Animation**:
  - Smooth macOS-like Genie morphing animation for flyout cards: windows dynamically expand and vacuum-shrink directly into the clicked dock pill button using `clip-path` morphing and cubic-bezier curves at 60 FPS.
- **Interactive Flyout Cards**:
  - **⚡ Quick Macros**: 1-click execution for `Win+D`, `Alt+Tab`, `Ctrl+Shift+Esc`, `Win+E`, `Ctrl+C`, `Ctrl+V`, `Win+L`, `Esc`.
  - **📋 Smart Clipboard**: Instant Unicode text dispatch with quick-fill chips for URLs and commands.
  - **🖱 Virtual Touch Trackpad**: Smooth touchpad zone with relative mouse motion (`mrel`), left/right click, and drag support.
  - **⌨ 5-Row On-Screen Keyboard**: Integrated virtual keyboard with illuminated modifier tags.
- **Auto-Dimming & Touch Ripples**:
  - Intelligent 3.5s auto-dimming of floating controls during active desktop work to prevent obscuring Windows taskbar/Start menu.
  - Cyan touch-ripple indicator providing instant physical visual confirmation of remote tap coordinates.

## [1.2.0] - 2026-09-03

### Added
- **Full-Size On-Screen Virtual Keyboard**:
  - Complete 5-row virtual keyboard for smartphones and tablets (Esc, numbers, QWERTY/ЙЦУКЕН letters, navigation arrows, Enter, Backspace, Tab, Caps, Shift, Ctrl, Alt, Win, Space).
  - One-tap language switcher (`🌐 RU / EN`) with full Cyrillic (ЙЦУКЕН) and Latin layouts.
  - Sticky modifiers (`Ctrl`, `Alt`, `Shift`, `Win`) for triggering keyboard shortcuts (`Ctrl+C`, `Ctrl+V`, `Alt+Tab`, `Win+D`) from mobile devices.
  - Direct text input field (`Ввести текст с телефона...`) with instantaneous clipboard paste (`pasteText` via `Ctrl+V`) for fast, error-free typing of whole sentences, passwords, and emojis from mobile software keyboards and voice input.
- **Complete Physical Keyboard Forwarding**:
  - Mapped all standard 104 keys (letters, digits, punctuation, brackets, math operators, functional keys F1-F12, navigation keys) directly from browser `e.code` to Windows virtual key codes.

## [1.1.0] - 2026-09-03

### Added
- **Full Keyboard & Typing Support**:
  - Direct Unicode keystroke injection (supports Russian Cyrillic, English, numbers, symbols, and any language without layout mismatch).
  - Virtual key code support for special keys (`Backspace`, `Enter`, `Tab`, `Escape`, `Delete`, `Arrow Keys`, `F1-F12`, `Win key`, modifiers).
  - Desktop keyboard integration: captures physical keyboard typing, shortcuts (`Ctrl+C`, `Ctrl+V`, `Ctrl+A`, `Ctrl+Z`), and special keys.
  - Mobile virtual keyboard toggle: added **"⌨ Клавиатура"** button to open the smartphone/tablet software keyboard.
  - Mobile Quick-Keys panel: handy touch buttons for `Esc`, `Tab`, `Win`, `Backspace`, and `Enter`.

## [1.0.2] - 2026-09-02

### Fixed
- **Mobile Browser Compatibility**:
  - Replaced button event bindings with robust form submission and numeric `inputmode` for mobile keyboards.
  - Fixed `localStorage` security exceptions in mobile private tabs/in-app webviews (Telegram, WhatsApp).
  - Added safe iOS Safari webkit fullscreen fallback and explicit `video.muted = true` handling for mobile autoplay policies.
  - Added visual loading indicator ("⏳ Вход...") on button tap.

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
