package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds persistent DeskRemote configuration options.
type Config struct {
	PIN              string `json:"pin"`
	TunnelToken      string `json:"tunnel_token"`
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatID   int64  `json:"telegram_chat_id"`
}

func getConfigPath() string {
	return filepath.Join(getExecutableDir(), "config.json")
}

// loadConfig reads config.json or returns default configuration, writing a template if missing.
func loadConfig() Config {
	cfg := Config{
		PIN:              "1234",
		TunnelToken:      "",
		TelegramBotToken: "",
		TelegramChatID:   0,
	}

	path := getConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			saveConfig(cfg)
			logToFile(fmt.Sprintf("[Config] Создан файл конфигурации по умолчанию: %s", path))
		}
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		logToFile(fmt.Sprintf("[Config] Предупреждение: ошибка парсинга config.json (%v), используются значения по умолчанию", err))
	}

	if cfg.PIN == "" {
		cfg.PIN = "1234"
	}

	return cfg
}

func saveConfig(cfg Config) {
	path := getConfigPath()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}
