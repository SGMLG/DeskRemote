package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	tunnelURLRegex = regexp.MustCompile(`https://[a-zA-Z0-9-.]+\.trycloudflare\.com`)
	logMu          sync.Mutex
	logFile        *os.File
	logOnce        sync.Once
)

func getLogFile() *os.File {
	logOnce.Do(func() {
		logPath := filepath.Join(getExecutableDir(), "deskremote.log")
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			logFile = f
		}
	})
	return logFile
}

// logToFile writes messages to deskremote.log in a thread-safe manner with timestamps.
func logToFile(msg string) {
	logMu.Lock()
	defer logMu.Unlock()

	f := getLogFile()
	if f != nil {
		timestamp := time.Now().Format("2006/01/02 15:04:05")
		_, _ = f.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, msg))
		_ = f.Sync()
	}
}

func getCloudflaredPath() string {
	if p := filepath.Join(getExecutableDir(), "cloudflared.exe"); fileExists(p) {
		if abs, err := filepath.Abs(p); err == nil { return abs }
		return p
	}
	if p := filepath.Join(".", "cloudflared.exe"); fileExists(p) {
		if abs, err := filepath.Abs(p); err == nil { return abs }
		return p
	}
	if p := `C:\DeskRemote\cloudflared.exe`; fileExists(p) {
		return p
	}
	if path, err := exec.LookPath("cloudflared"); err == nil {
		return path
	}
	return "cloudflared"
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// checkDependencies verifies that required binaries exist.
func checkDependencies() {
	ffmpegPath := getFFmpegPath()
	if !fileExists(ffmpegPath) {
		logToFile("[ВНИМАНИЕ] ffmpeg.exe не найден! Захват экрана работать не будет.")
	} else {
		logToFile(fmt.Sprintf("[Deps] ffmpeg обнаружен: %s", ffmpegPath))
	}

	cloudflaredPath := getCloudflaredPath()
	if !fileExists(cloudflaredPath) {
		logToFile("[ВНИМАНИЕ] cloudflared.exe не найден! Удаленный туннель Cloudflare запустить не удастся.")
	} else {
		logToFile(fmt.Sprintf("[Deps] cloudflared обнаружен: %s", cloudflaredPath))
	}
}

func startCloudflareTunnel(ctx context.Context, token string, onURLReady func(string)) {
	cloudflaredPath := getCloudflaredPath()
	logToFile(fmt.Sprintf("[Tunnel] Запуск %s", cloudflaredPath))

	var args []string
	if token != "" {
		args = []string{"tunnel", "run", "--token", token}
	} else {
		args = []string{"tunnel", "--url", "http://localhost:8080"}
	}

	cmd := exec.CommandContext(ctx, cloudflaredPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logToFile(fmt.Sprintf("[Tunnel] Ошибка StderrPipe: %v", err))
		return
	}
	stdout, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		logToFile(fmt.Sprintf("[Tunnel] Ошибка Start cloudflared: %v (проверьте наличие бинарника)", err))
		return
	}
	logToFile("[Tunnel] Процесс cloudflared запущен!")

	handleScan := func(scanner *bufio.Scanner, name string) {
		for scanner.Scan() {
			line := scanner.Text()
			logToFile(fmt.Sprintf("[%s] %s", name, line))
			if strings.Contains(line, "trycloudflare.com") {
				match := tunnelURLRegex.FindString(line)
				if match != "" && onURLReady != nil {
					logToFile(fmt.Sprintf("[Tunnel] Найдена ссылка: %s", match))
					onURLReady(match)
				}
			}
		}
	}

	go handleScan(bufio.NewScanner(stderr), "stderr")
	if stdout != nil {
		go handleScan(bufio.NewScanner(stdout), "stdout")
	}

	<-ctx.Done()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}
