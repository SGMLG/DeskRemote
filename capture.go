package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfreader"
)

func getExecutableDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

func startScreenCapture(ctx context.Context, videoTrack *webrtc.TrackLocalStaticSample) {
	ffmpegPath := filepath.Join(getExecutableDir(), "ffmpeg.exe")
	if _, err := os.Stat(ffmpegPath); err != nil {
		ffmpegPath = "ffmpeg"
	}

	logToFile("[Capture] Запуск ffmpeg для захвата экрана с фильтром yuv420p...")

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-f", "gdigrab",
		"-framerate", "30",
		"-draw_mouse", "1",
		"-i", "desktop",
		"-vf", "format=yuv420p",
		"-c:v", "libvpx",
		"-b:v", "2M",
		"-crf", "30",
		"-deadline", "realtime",
		"-cpu-used", "5",
		"-an",
		"-f", "ivf", "pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logToFile(fmt.Sprintf("[Capture] Ошибка создания stdout пайпа: %v", err))
		return
	}

	stderr, _ := cmd.StderrPipe()
	if stderr != nil {
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				logToFile("[ffmpeg] " + line)
			}
		}()
	}

	if err := cmd.Start(); err != nil {
		logToFile(fmt.Sprintf("[Capture] Не удалось запустить ffmpeg: %v", err))
		return
	}

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	ivf, header, err := ivfreader.NewWith(stdout)
	if err != nil {
		logToFile(fmt.Sprintf("[Capture] Ошибка IVF парсера: %v", err))
		return
	}
	logToFile(fmt.Sprintf("[Capture] Поток успешно запущен: %dx%d (%s)", header.Width, header.Height, header.FourCC))

	for {
		select {
		case <-ctx.Done():
			logToFile("[Capture] Захват остановлен (context done)")
			return
		default:
			frame, _, err := ivf.ParseNextFrame()
			if err != nil {
				if errors.Is(err, io.EOF) {
					logToFile("[Capture] Поток завершен (EOF)")
					return
				}
				logToFile(fmt.Sprintf("[Capture] Ошибка чтения кадра: %v", err))
				return
			}
			_ = videoTrack.WriteSample(media.Sample{
				Data:     frame,
				Duration: time.Millisecond * 33,
			})
		}
	}
}
