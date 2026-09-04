# DeskRemote Project Memory

## Правила репозитория
- **Версионирование**: НЕ повышать версию проекта (`VERSION`, `version.go`, `CHANGELOG.md`) после каждой отдельной правки. Версия меняется только по прямому указанию пользователя при выпуске полноценного релиза.
- **Стек**: Go 1.22, WebRTC (Pion v4), Cloudflare Tunnel, FFmpeg, Windows Win32 API (`user32.dll`, `kernel32.dll`), Vanilla JS / CSS3 (встроен в бинарник через `//go:embed index.html`).
- **Тесты**: E2E сценарии в `tests/test_e2e.py` (Playwright Chromium).
