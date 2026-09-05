package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// tgUpdate represents incoming Telegram update structure.
type tgUpdate struct {
	UpdateID int        `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int     `json:"message_id"`
	From      *tgUser `json:"from"`
	Chat      tgChat  `json:"chat"`
	Text      string  `json:"text"`
}

type tgUser struct {
	ID       int64  `json:"id"`
	UserName string `json:"username"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

// ReplyKeyboardMarkup represents a Telegram custom reply keyboard.
type ReplyKeyboardMarkup struct {
	Keyboard       [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard bool               `json:"resize_keyboard"`
	IsPersistent   bool               `json:"is_persistent"`
}

type KeyboardButton struct {
	Text string `json:"text"`
}

// TelegramBot coordinates remote control of DeskRemote via Telegram Bot API.
type TelegramBot struct {
	token         string
	allowedChatID int64
	pin           string
	startTime     time.Time
	currentURL    string
	urlMu         sync.RWMutex
	client        *http.Client
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewTelegramBot constructs a new bot instance.
func NewTelegramBot(token string, allowedChatID int64, pin string) *TelegramBot {
	ctx, cancel := context.WithCancel(context.Background())
	return &TelegramBot{
		token:         strings.TrimSpace(token),
		allowedChatID: allowedChatID,
		pin:           pin,
		startTime:     time.Now(),
		client:        &http.Client{Timeout: 35 * time.Second},
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start launches the Telegram long-polling loop in a background goroutine.
func (b *TelegramBot) Start() {
	if b.token == "" {
		logToFile("[TelegramBot] Токен бота не задан, бот не будет запущен")
		return
	}

	logToFile(fmt.Sprintf("[TelegramBot] Запуск Telegram-бота (авторизованный Chat ID: %d)...", b.allowedChatID))
	go b.pollLoop()
}

// Stop cleanly stops the bot background routines.
func (b *TelegramBot) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

// SetURL updates current active Cloudflare tunnel URL.
func (b *TelegramBot) SetURL(url string) {
	b.urlMu.Lock()
	b.currentURL = url
	b.urlMu.Unlock()
}

// GetURL returns current active Cloudflare tunnel URL.
func (b *TelegramBot) GetURL() string {
	b.urlMu.RLock()
	defer b.urlMu.RUnlock()
	return b.currentURL
}

// NotifyTunnelReady sends automatic notification with fresh link and PIN to the owner.
func (b *TelegramBot) NotifyTunnelReady(url string, pin string) {
	b.SetURL(url)
	if b.allowedChatID == 0 {
		return
	}

	msg := fmt.Sprintf(
		"🟢 <b>DeskRemote готов к подключению!</b>\n\n"+
			"🔗 <b>Ссылка:</b> %s\n"+
			"🔑 <b>PIN-код:</b> <code>%s</code>\n"+
			"⏱ <i>Сервер запущен и транслирует экран в реальном времени.</i>",
		url, pin,
	)

	_ = b.sendMessage(b.allowedChatID, msg, b.mainKeyboard())
}

func (b *TelegramBot) mainKeyboard() ReplyKeyboardMarkup {
	return ReplyKeyboardMarkup{
		Keyboard: [][]KeyboardButton{
			{{Text: "🔗 Ссылка"}, {Text: "📸 Скриншот"}},
			{{Text: "📊 Статус"}, {Text: "🔒 Заблокировать"}},
			{{Text: "⚡ Питание"}},
		},
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

func (b *TelegramBot) powerKeyboard() ReplyKeyboardMarkup {
	return ReplyKeyboardMarkup{
		Keyboard: [][]KeyboardButton{
			{{Text: "⚠️ Выключить ПК (30с)"}},
			{{Text: "🔄 Перезагрузить ПК (30с)"}},
			{{Text: "❌ Отменить выключение"}},
			{{Text: "🔙 Главное меню"}},
		},
		ResizeKeyboard: true,
		IsPersistent:   true,
	}
}

func (b *TelegramBot) pollLoop() {
	offset := 0
	for {
		select {
		case <-b.ctx.Done():
			logToFile("[TelegramBot] Цикл опроса завершен")
			return
		default:
		}

		updates, nextOffset, err := b.getUpdates(offset)
		if err != nil {
			if b.ctx.Err() != nil {
				return
			}
			time.Sleep(3 * time.Second)
			continue
		}

		offset = nextOffset
		for _, u := range updates {
			if u.Message == nil {
				continue
			}
			b.handleMessage(u.Message)
		}
	}
}

func (b *TelegramBot) getUpdates(offset int) ([]tgUpdate, int, error) {
	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=25", b.token, offset)
	req, err := http.NewRequestWithContext(b.ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, offset, err
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, offset, err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, offset, err
	}

	maxOffset := offset
	for _, u := range result.Result {
		if u.UpdateID >= maxOffset {
			maxOffset = u.UpdateID + 1
		}
	}

	return result.Result, maxOffset, nil
}

func (b *TelegramBot) handleMessage(msg *tgMessage) {
	chatID := msg.Chat.ID
	fromUser := "unknown"
	if msg.From != nil {
		fromUser = msg.From.UserName
	}

	// Security: Whitelist Check
	if b.allowedChatID != 0 && chatID != b.allowedChatID {
		logToFile(fmt.Sprintf("[TelegramBot] Отклонено неавторизованное сообщение от ChatID: %d (@%s)", chatID, fromUser))
		_ = b.sendMessage(chatID, "⛔ <b>Доступ запрещен.</b>\nЭтот бот привязан к личному ПК владельца DeskRemote.", nil)
		return
	}

	text := strings.TrimSpace(msg.Text)
	logToFile(fmt.Sprintf("[TelegramBot] Команда от владельца: %s", text))

	switch strings.ToLower(text) {
	case "/start", "/help", "меню", "🔙 главное меню":
		welcome := "👋 <b>Панель управления DeskRemote</b>\n\n" +
			"Используйте кнопки меню для удаленного контроля компьютера."
		_ = b.sendMessage(chatID, welcome, b.mainKeyboard())

	case "/link", "🔗 ссылка":
		url := b.GetURL()
		if url == "" {
			_ = b.sendMessage(chatID, "⏳ Туннель Cloudflare инициализируется, подождите пару секунд...", b.mainKeyboard())
			return
		}
		reply := fmt.Sprintf(
			"🔗 <b>Актуальная ссылка DeskRemote:</b>\n%s\n\n"+
				"🔑 <b>PIN-код:</b> <code>%s</code>",
			url, b.pin,
		)
		_ = b.sendMessage(chatID, reply, b.mainKeyboard())

	case "/screen", "📸 скриншот":
		_ = b.sendMessage(chatID, "📸 <i>Делаю снимок экрана...</i>", nil)
		imgData, err := captureScreenshotJPEG()
		if err != nil {
			_ = b.sendMessage(chatID, fmt.Sprintf("❌ Ошибка захвата экрана: %v\n<i>(Возможно, экран Windows заблокирован)</i>", err), b.mainKeyboard())
			return
		}
		caption := fmt.Sprintf("🖥 Снимок экрана (%s)", time.Now().Format("15:04:05"))
		if err := b.sendPhoto(chatID, imgData, caption); err != nil {
			_ = b.sendMessage(chatID, fmt.Sprintf("❌ Ошибка отправки фото в Telegram: %v", err), b.mainKeyboard())
		}

	case "/status", "📊 статус":
		uptime := time.Since(b.startTime).Round(time.Second)
		activeClients := broadcaster.GetActiveClientsCount()
		
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memUsageMB := float64(m.Alloc) / 1024 / 1024

		statusMsg := fmt.Sprintf(
			"📊 <b>Статус сервера DeskRemote</b>\n\n"+
				"⏱ <b>Время работы:</b> %s\n"+
				"👥 <b>Активных клиентов WebRTC:</b> %d\n"+
				"🧠 <b>Память (RAM):</b> %.1f МБ\n"+
				"🔗 <b>Туннель:</b> %s",
			uptime, activeClients, memUsageMB, b.GetURL(),
		)
		_ = b.sendMessage(chatID, statusMsg, b.mainKeyboard())

	case "/lock", "🔒 заблокировать":
		logToFile("[TelegramBot] Блокировка экрана по команде из Telegram")
		procLockWorkStation.Call()
		_ = b.sendMessage(chatID, "🔒 <b>Экран Windows заблокирован.</b>", b.mainKeyboard())

	case "/power", "⚡ питание":
		powerPrompt := "⚡ <b>Управление питанием компьютера:</b>\n\n" +
			"Выберите действие (для безопасности задана задержка 30 секунд):"
		_ = b.sendMessage(chatID, powerPrompt, b.powerKeyboard())

	case "⚠️ выключить пк (30с)", "/shutdown":
		cmd := exec.Command("shutdown", "/s", "/t", "30", "/c", "Выключение по запросу через DeskRemote Telegram Bot")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
		_ = b.sendMessage(chatID, "⚠️ <b>Компьютер выключится через 30 секунд!</b>\nЕсли передумали, нажмите «❌ Отменить выключение».", b.powerKeyboard())

	case "🔄 перезагрузить пк (30с)", "/reboot":
		cmd := exec.Command("shutdown", "/r", "/t", "30", "/c", "Перезагрузка по запросу через DeskRemote Telegram Bot")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
		_ = b.sendMessage(chatID, "🔄 <b>Компьютер перезагрузится через 30 секунд!</b>\nЕсли передумали, нажмите «❌ Отменить выключение».", b.powerKeyboard())

	case "❌ отменить выключение", "/abort":
		cmd := exec.Command("shutdown", "/a")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
		_ = b.sendMessage(chatID, "✅ <b>Запланированное выключение/перезагрузка отменены.</b>", b.mainKeyboard())

	default:
		_ = b.sendMessage(chatID, "❓ Неизвестная команда. Выберите действие на клавиатуре:", b.mainKeyboard())
	}
}

func (b *TelegramBot) sendMessage(chatID int64, text string, replyMarkup any) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.token)
	req, err := http.NewRequestWithContext(b.ctx, "POST", reqURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (b *TelegramBot) sendPhoto(chatID int64, photoData []byte, caption string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	_ = writer.WriteField("chat_id", strconv.FormatInt(chatID, 10))
	if caption != "" {
		_ = writer.WriteField("caption", caption)
	}

	part, err := writer.CreateFormFile("photo", "screenshot.jpg")
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, bytes.NewReader(photoData)); err != nil {
		return err
	}
	_ = writer.Close()

	reqURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", b.token)
	req, err := http.NewRequestWithContext(b.ctx, "POST", reqURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// captureScreenshotJPEG captures a high-quality single frame of the Windows desktop using FFmpeg.
func captureScreenshotJPEG() ([]byte, error) {
	ffmpegPath := getFFmpegPath()
	if !fileExists(ffmpegPath) {
		return nil, fmt.Errorf("ffmpeg.exe не найден (%s)", ffmpegPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("deskremote_snap_%d.jpg", time.Now().UnixNano()))
	defer os.Remove(tempPath)

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-f", "gdigrab",
		"-draw_mouse", "1",
		"-i", "desktop",
		"-vframes", "1",
		"-q:v", "3",
		"-y",
		tempPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ошибка выполнения ffmpeg: %w", err)
	}

	data, err := os.ReadFile(tempPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения скриншота: %w", err)
	}
	return data, nil
}
