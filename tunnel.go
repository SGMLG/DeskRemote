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
)

var tunnelURLRegex = regexp.MustCompile(`https://[a-zA-Z0-9-.]+\.trycloudflare\.com`)

func logToFile(msg string) {
	logPath := filepath.Join(getExecutableDir(), "deskremote.log")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		_, _ = f.WriteString(msg + "\n")
	}
}

func startCloudflareTunnel(ctx context.Context, token string, onURLReady func(string)) {
	cloudflaredPath := filepath.Join(getExecutableDir(), "cloudflared.exe")
	if _, err := os.Stat(cloudflaredPath); err != nil {
		cloudflaredPath = "cloudflared"
	}
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
		logToFile(fmt.Sprintf("[Tunnel] Ошибка Start: %v", err))
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
}
