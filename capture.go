package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/h264reader"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
)

func getExecutableDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

func getFFmpegPath() string {
	// 1. Next to executable
	ffmpegPath := filepath.Join(getExecutableDir(), "ffmpeg.exe")
	if _, err := os.Stat(ffmpegPath); err == nil {
		if abs, err := filepath.Abs(ffmpegPath); err == nil {
			return abs
		}
		return ffmpegPath
	}
	// 2. In current working directory
	cwdFfmpeg := filepath.Join(".", "ffmpeg.exe")
	if _, err := os.Stat(cwdFfmpeg); err == nil {
		if abs, err := filepath.Abs(cwdFfmpeg); err == nil {
			return abs
		}
		return cwdFfmpeg
	}
	// 3. In standard install location
	instFfmpeg := `C:\DeskRemote\ffmpeg.exe`
	if _, err := os.Stat(instFfmpeg); err == nil {
		return instFfmpeg
	}
	// 4. In system PATH
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path
	}
	return "ffmpeg"
}

// VideoEncoderConfig defines the chosen video encoder and its FFmpeg arguments.
type VideoEncoderConfig struct {
	Name    string
	Encoder string
	Args    []string
}

var (
	encoderOnce         sync.Once
	detectedVideoConfig VideoEncoderConfig
)

func testEncoder(ffmpegPath, encoder string, extraArgs ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmdArgs := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=s=64x64:d=0.1",
		"-c:v", encoder,
	}
	cmdArgs = append(cmdArgs, extraArgs...)
	cmdArgs = append(cmdArgs, "-frames:v", "1", "-f", "null", "-")

	cmd := exec.CommandContext(ctx, ffmpegPath, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run() == nil
}

func detectBestVideoEncoder(ffmpegPath string) VideoEncoderConfig {
	encoderOnce.Do(func() {
		logToFile("[Capture] Определение оптимального видеоэнкодера GPU/CPU...")

		// 1. AMD AMF (Hardware - Radeon RX 400/500/Vega/Navi/RDNA)
		// Note: h264_amf requires 'constrained_baseline' or 'main' as profile value.
		if testEncoder(ffmpegPath, "h264_amf", "-profile:v", "constrained_baseline") {
			logToFile("[Capture] Обнаружен аппаратный энкодер: AMD AMF (h264_amf, constrained_baseline). Включаем аппаратное ускорение на GPU!")
			detectedVideoConfig = VideoEncoderConfig{
				Name:    "AMD AMF (Hardware)",
				Encoder: "h264_amf",
				Args: []string{
					"-c:v", "h264_amf",
					"-quality", "speed",
					"-rc", "cbr",
					"-b:v", "3M",
					"-maxrate", "4M",
					"-bufsize", "2M",
					"-g", "60",
					"-bf", "0",
					"-header_spacing", "1",
					"-forced_idr", "1",
					"-profile:v", "constrained_baseline",
					"-pix_fmt", "yuv420p",
					"-f", "h264", "pipe:1",
				},
			}
			return
		} else if testEncoder(ffmpegPath, "h264_amf", "-profile:v", "main") {
			logToFile("[Capture] Обнаружен аппаратный энкодер: AMD AMF (h264_amf, main). Включаем аппаратное ускорение на GPU!")
			detectedVideoConfig = VideoEncoderConfig{
				Name:    "AMD AMF (Hardware)",
				Encoder: "h264_amf",
				Args: []string{
					"-c:v", "h264_amf",
					"-quality", "speed",
					"-rc", "cbr",
					"-b:v", "3M",
					"-maxrate", "4M",
					"-bufsize", "2M",
					"-g", "60",
					"-bf", "0",
					"-header_spacing", "1",
					"-forced_idr", "1",
					"-profile:v", "main",
					"-pix_fmt", "yuv420p",
					"-f", "h264", "pipe:1",
				},
			}
			return
		}

		// 2. NVIDIA NVENC (Hardware - GeForce GTX/RTX)
		if testEncoder(ffmpegPath, "h264_nvenc", "-profile:v", "baseline") {
			logToFile("[Capture] Обнаружен аппаратный энкодер: NVIDIA NVENC (h264_nvenc). Включаем аппаратное ускорение на GPU!")
			detectedVideoConfig = VideoEncoderConfig{
				Name:    "NVIDIA NVENC (Hardware)",
				Encoder: "h264_nvenc",
				Args: []string{
					"-c:v", "h264_nvenc",
					"-preset", "p1",
					"-tune", "ull",
					"-rc", "cbr",
					"-b:v", "3M",
					"-maxrate", "4M",
					"-bufsize", "2M",
					"-g", "60",
					"-bf", "0",
					"-forced-idr", "1",
					"-repeat-headers", "1",
					"-profile:v", "baseline",
					"-pix_fmt", "yuv420p",
					"-f", "h264", "pipe:1",
				},
			}
			return
		}

		// 3. Intel QuickSync (Hardware - Intel Core iGPU)
		if testEncoder(ffmpegPath, "h264_qsv", "-profile:v", "baseline") {
			logToFile("[Capture] Обнаружен аппаратный энкодер: Intel QuickSync (h264_qsv). Включаем аппаратное ускорение!")
			detectedVideoConfig = VideoEncoderConfig{
				Name:    "Intel QSV (Hardware)",
				Encoder: "h264_qsv",
				Args: []string{
					"-c:v", "h264_qsv",
					"-preset", "veryfast",
					"-b:v", "3M",
					"-maxrate", "4M",
					"-bufsize", "2M",
					"-g", "60",
					"-bf", "0",
					"-profile:v", "baseline",
					"-pix_fmt", "yuv420p",
					"-f", "h264", "pipe:1",
				},
			}
			return
		}

		// 4. Software libx264 fallback
		logToFile("[Capture] Аппаратные энкодеры GPU не найдены. Используем оптимизированный x264 (CPU)...")
		detectedVideoConfig = VideoEncoderConfig{
			Name:    "libx264 (CPU Ultrafast)",
			Encoder: "libx264",
			Args: []string{
				"-c:v", "libx264",
				"-preset", "ultrafast",
				"-tune", "zerolatency",
				"-x264-params", "repeat-headers=1",
				"-b:v", "2500k",
				"-maxrate", "3000k",
				"-bufsize", "1000k",
				"-g", "60",
				"-bf", "0",
				"-profile:v", "baseline",
				"-pix_fmt", "yuv420p",
				"-f", "h264", "pipe:1",
			},
		}
	})
	return detectedVideoConfig
}

// ScreenBroadcaster implements a thread-safe singleton screen capture service.
// A single FFmpeg process captures the desktop and broadcasts frames to all active WebRTC tracks.
// When clients disconnect or reload (F5), a 3-second grace period prevents wasteful process restarts.
type ScreenBroadcaster struct {
	mu         sync.Mutex
	tracks     map[*webrtc.TrackLocalStaticSample]struct{}
	running    bool
	generation int
	cancelFunc context.CancelFunc
	idleTimer  *time.Timer
	cmd        *exec.Cmd
	sps        []byte
	pps        []byte
}

var broadcaster = &ScreenBroadcaster{
	tracks: make(map[*webrtc.TrackLocalStaticSample]struct{}),
}

// GetActiveClientsCount returns the number of active video clients.
func (b *ScreenBroadcaster) GetActiveClientsCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.tracks)
}

// RegisterTrack adds a track to receive screen capture frames and starts FFmpeg if needed.
func (b *ScreenBroadcaster) RegisterTrack(track *webrtc.TrackLocalStaticSample) {
	b.mu.Lock()

	if b.idleTimer != nil {
		b.idleTimer.Stop()
		b.idleTimer = nil
	}

	var spsCopy, ppsCopy []byte
	if b.running {
		if len(b.sps) > 0 && len(b.pps) > 0 {
			spsCopy = append([]byte(nil), b.sps...)
			ppsCopy = append([]byte(nil), b.pps...)
		}
	} else {
		b.generation++
		gen := b.generation
		ctx, cancel := context.WithCancel(context.Background())
		b.cancelFunc = cancel
		b.running = true
		go b.captureLoop(ctx, gen)
	}

	// Deliver cached SPS/PPS headers immediately to the new track BEFORE registering it
	// for broadcasts, guaranteeing that iOS VideoToolbox receives codec parameter sets
	// before any subsequent stream frames (IDR or non-IDR).
	if len(spsCopy) > 0 && len(ppsCopy) > 0 {
		_ = track.WriteSample(media.Sample{Data: spsCopy, Duration: 0})
		_ = track.WriteSample(media.Sample{Data: ppsCopy, Duration: 0})
	}

	b.tracks[track] = struct{}{}
	logToFile(fmt.Sprintf("[VideoBroadcaster] Видеотрек зарегистрирован (всего активных: %d)", len(b.tracks)))
	b.mu.Unlock()
}

// UnregisterTrack removes a track from receiving frames.
// If no tracks remain, a 3-second idle timer is started before stopping FFmpeg.
func (b *ScreenBroadcaster) UnregisterTrack(track *webrtc.TrackLocalStaticSample) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.tracks, track)
	logToFile(fmt.Sprintf("[VideoBroadcaster] Видеотрек отключен (осталось активных: %d)", len(b.tracks)))

	if len(b.tracks) == 0 && b.running {
		if b.idleTimer != nil {
			b.idleTimer.Stop()
		}
		b.idleTimer = time.AfterFunc(3*time.Second, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if len(b.tracks) == 0 && b.running {
				logToFile("[VideoBroadcaster] Нет активных клиентов более 3 секунд. Остановка FFmpeg...")
				b.stopInternal()
			}
		})
	}
}

// Stop terminates the screen capture process immediately.
func (b *ScreenBroadcaster) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopInternal()
}

func (b *ScreenBroadcaster) stopInternal() {
	if b.idleTimer != nil {
		b.idleTimer.Stop()
		b.idleTimer = nil
	}
	if b.cancelFunc != nil {
		b.cancelFunc()
		b.cancelFunc = nil
	}
	if b.cmd != nil {
		if b.cmd.Process != nil {
			_ = b.cmd.Process.Kill()
		}
		b.cmd = nil
	}
	b.running = false
	b.tracks = make(map[*webrtc.TrackLocalStaticSample]struct{})
	b.sps = nil
	b.pps = nil
	logToFile("[VideoBroadcaster] Захват экрана полностью остановлен")
}

func (b *ScreenBroadcaster) broadcastSample(sample media.Sample) {
	b.mu.Lock()
	if len(b.tracks) == 0 {
		b.mu.Unlock()
		return
	}
	activeTracks := make([]*webrtc.TrackLocalStaticSample, 0, len(b.tracks))
	for track := range b.tracks {
		activeTracks = append(activeTracks, track)
	}
	b.mu.Unlock()

	for _, track := range activeTracks {
		_ = track.WriteSample(sample)
	}
}

func (b *ScreenBroadcaster) captureLoop(ctx context.Context, gen int) {
	defer func() {
		b.mu.Lock()
		if b.generation == gen {
			b.running = false
			b.cmd = nil
			b.cancelFunc = nil
			b.sps = nil
			b.pps = nil
		}
		b.mu.Unlock()
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		b.mu.Lock()
		if b.generation != gen {
			b.mu.Unlock()
			return
		}
		activeCount := len(b.tracks)
		b.mu.Unlock()

		if activeCount == 0 {
			return
		}

		b.runCaptureSession(ctx, gen)

		if ctx.Err() != nil {
			return
		}

		b.mu.Lock()
		if b.generation != gen {
			b.mu.Unlock()
			return
		}
		activeCount = len(b.tracks)
		b.mu.Unlock()

		if activeCount == 0 {
			return
		}

		logToFile(fmt.Sprintf("[Capture] Поток завершился (EOF/ошибка). Повторный запуск через 2 секунды (активных клиентов: %d)...", activeCount))

		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (b *ScreenBroadcaster) runCaptureSession(ctx context.Context, gen int) {
	ffmpegPath := getFFmpegPath()
	encoderConfig := detectBestVideoEncoder(ffmpegPath)

	logToFile(fmt.Sprintf("[Capture] Запуск захвата экрана (60 FPS, энкодер: %s, %s)...", encoderConfig.Name, ffmpegPath))

	baseArgs := []string{
		"-f", "gdigrab",
		"-framerate", "60",
		"-draw_mouse", "1",
		"-i", "desktop",
	}
	cmdArgs := append(baseArgs, encoderConfig.Args...)

	cmd := exec.CommandContext(ctx, ffmpegPath, cmdArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

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
				if len(line) > 0 {
					logToFile("[ffmpeg-video] " + line)
				}
			}
		}()
	}

	if err := cmd.Start(); err != nil {
		logToFile(fmt.Sprintf("[Capture] Не удалось запустить ffmpeg: %v", err))
		return
	}

	b.mu.Lock()
	if b.generation != gen || ctx.Err() != nil {
		b.mu.Unlock()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return
	}
	b.cmd = cmd
	b.mu.Unlock()

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		b.mu.Lock()
		if b.cmd == cmd {
			b.cmd = nil
		}
		b.mu.Unlock()
	}()

	h264, err := h264reader.NewReader(stdout)
	if err != nil {
		logToFile(fmt.Sprintf("[Capture] Ошибка H264 парсера: %v", err))
		return
	}
	logToFile("[Capture] H.264 поток успешно запущен (60 FPS)")

	for {
		if ctx.Err() != nil {
			logToFile("[Capture] Захват завершен по контексту")
			return
		}

		nal, err := h264.NextNAL()
		if err != nil {
			if errors.Is(err, io.EOF) {
				logToFile("[Capture] Поток завершен (EOF)")
			} else if ctx.Err() == nil {
				logToFile(fmt.Sprintf("[Capture] Ошибка чтения кадра: %v", err))
			}
			return
		}

		if nal.UnitType == h264reader.NalUnitTypeSPS {
			b.mu.Lock()
			b.sps = make([]byte, len(nal.Data))
			copy(b.sps, nal.Data)
			b.mu.Unlock()
		} else if nal.UnitType == h264reader.NalUnitTypePPS {
			b.mu.Lock()
			b.pps = make([]byte, len(nal.Data))
			copy(b.pps, nal.Data)
			b.mu.Unlock()
		}

		duration := time.Duration(0)
		if nal.UnitType == h264reader.NalUnitTypeCodedSliceIdr || nal.UnitType == h264reader.NalUnitTypeCodedSliceNonIdr {
			duration = 16666 * time.Microsecond
		}

		b.broadcastSample(media.Sample{
			Data:     nal.Data,
			Duration: duration,
		})
	}
}

// AudioBroadcaster implements a thread-safe singleton audio capture and distribution service.
// It captures Windows system audio via DirectShow, encodes with Opus, and broadcasts to clients.
type AudioBroadcaster struct {
	mu         sync.Mutex
	tracks     map[*webrtc.TrackLocalStaticSample]struct{}
	running    bool
	generation int
	cancelFunc context.CancelFunc
	idleTimer  *time.Timer
	cmd        *exec.Cmd
}

var audioBroadcaster = &AudioBroadcaster{
	tracks: make(map[*webrtc.TrackLocalStaticSample]struct{}),
}

// RegisterTrack adds a track to receive audio frames and starts FFmpeg if needed.
func (b *AudioBroadcaster) RegisterTrack(track *webrtc.TrackLocalStaticSample) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.idleTimer != nil {
		b.idleTimer.Stop()
		b.idleTimer = nil
	}

	b.tracks[track] = struct{}{}
	logToFile(fmt.Sprintf("[AudioBroadcaster] Аудиотрек зарегистрирован (всего активных: %d)", len(b.tracks)))

	if !b.running {
		b.generation++
		gen := b.generation
		ctx, cancel := context.WithCancel(context.Background())
		b.cancelFunc = cancel
		b.running = true
		go b.captureLoop(ctx, gen)
	}
}

// UnregisterTrack removes a track from receiving audio frames.
func (b *AudioBroadcaster) UnregisterTrack(track *webrtc.TrackLocalStaticSample) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.tracks, track)
	logToFile(fmt.Sprintf("[AudioBroadcaster] Аудиотрек отключен (осталось активных: %d)", len(b.tracks)))

	if len(b.tracks) == 0 && b.running {
		if b.idleTimer != nil {
			b.idleTimer.Stop()
		}
		b.idleTimer = time.AfterFunc(3*time.Second, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if len(b.tracks) == 0 && b.running {
				logToFile("[AudioBroadcaster] Нет активных клиентов более 3 секунд. Остановка аудиозахвата...")
				b.stopInternal()
			}
		})
	}
}

// Stop terminates the audio capture process immediately.
func (b *AudioBroadcaster) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopInternal()
}

func (b *AudioBroadcaster) stopInternal() {
	if b.idleTimer != nil {
		b.idleTimer.Stop()
		b.idleTimer = nil
	}
	if b.cancelFunc != nil {
		b.cancelFunc()
		b.cancelFunc = nil
	}
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Kill()
	}
	b.running = false
	b.tracks = make(map[*webrtc.TrackLocalStaticSample]struct{})
	logToFile("[AudioBroadcaster] Захват аудио полностью остановлен")
}

func (b *AudioBroadcaster) broadcastSample(sample media.Sample) {
	b.mu.Lock()
	if len(b.tracks) == 0 {
		b.mu.Unlock()
		return
	}
	activeTracks := make([]*webrtc.TrackLocalStaticSample, 0, len(b.tracks))
	for track := range b.tracks {
		activeTracks = append(activeTracks, track)
	}
	b.mu.Unlock()

	for _, track := range activeTracks {
		_ = track.WriteSample(sample)
	}
}

// detectAudioDevice finds the best DirectShow audio device name or alternative ID.
func detectAudioDevice(ffmpegPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()

	scanner := bufio.NewScanner(&stderr)
	var devices []string
	var altNames []string

	reName := regexp.MustCompile(`"([^"]+)"\s+\(audio\)`)
	reAlt := regexp.MustCompile(`Alternative name "(@device_[^"]+)"`)

	for scanner.Scan() {
		line := scanner.Text()
		if m := reName.FindStringSubmatch(line); len(m) > 1 {
			devices = append(devices, m[1])
		}
		if m := reAlt.FindStringSubmatch(line); len(m) > 1 {
			altNames = append(altNames, m[1])
		}
	}

	if len(altNames) > 0 {
		logToFile(fmt.Sprintf("[Audio] Найдено аудиоустройство DirectShow (ID: %s)", altNames[0]))
		return altNames[0]
	}
	if len(devices) > 0 {
		logToFile(fmt.Sprintf("[Audio] Найдено аудиоустройство DirectShow: %s", devices[0]))
		return devices[0]
	}

	logToFile("[Audio] Звуковые устройства DirectShow не обнаружены")
	return ""
}

func (b *AudioBroadcaster) captureLoop(ctx context.Context, gen int) {
	ffmpegPath := getFFmpegPath()
	audioDevice := detectAudioDevice(ffmpegPath)

	if audioDevice == "" {
		logToFile("[Audio] Аудиозахват пропущен: аудиоустройство не найдено")
		b.mu.Lock()
		if b.generation == gen {
			b.running = false
		}
		b.mu.Unlock()
		return
	}

	logToFile(fmt.Sprintf("[Audio] Запуск захвата звука (%s -> Opus 64k)...", audioDevice))

	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-hide_banner",
		"-f", "dshow",
		"-i", "audio="+audioDevice,
		"-vn",
		"-c:a", "libopus",
		"-b:a", "64k",
		"-vbr", "on",
		"-application", "lowdelay",
		"-frame_duration", "20",
		"-page_duration", "20000",
		"-f", "ogg", "pipe:1",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	b.mu.Lock()
	b.cmd = cmd
	b.mu.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logToFile(fmt.Sprintf("[Audio] Ошибка создания stdout пайпа: %v", err))
		b.mu.Lock()
		if b.generation == gen {
			b.running = false
		}
		b.mu.Unlock()
		return
	}

	stderr, _ := cmd.StderrPipe()
	if stderr != nil {
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				if len(line) > 0 {
					logToFile("[ffmpeg-audio] " + line)
				}
			}
		}()
	}

	if err := cmd.Start(); err != nil {
		logToFile(fmt.Sprintf("[Audio] Не удалось запустить ffmpeg для аудио: %v", err))
		b.mu.Lock()
		if b.generation == gen {
			b.running = false
		}
		b.mu.Unlock()
		return
	}

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		b.mu.Lock()
		if b.generation == gen {
			b.running = false
			b.cmd = nil
			b.cancelFunc = nil
		}
		b.mu.Unlock()
	}()

	ogg, _, err := oggreader.NewWith(stdout)
	if err != nil {
		logToFile(fmt.Sprintf("[Audio] Ошибка Ogg парсера: %v", err))
		return
	}
	logToFile("[Audio] Opus аудиопоток успешно инициализирован")

	var lastGranule uint64
	firstPage := true
	for {
		if ctx.Err() != nil {
			logToFile("[Audio] Захват аудио завершен по контексту")
			return
		}

		pageData, pageHeader, err := ogg.ParseNextPage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				logToFile("[Audio] Аудиопоток завершен (EOF)")
			} else if ctx.Err() == nil {
				logToFile(fmt.Sprintf("[Audio] Ошибка чтения страницы Ogg: %v", err))
			}
			return
		}

		sampleDuration := 20 * time.Millisecond
		if firstPage {
			// Skip duration calculation on the first page to avoid
			// a huge duration from initial GranulePosition offset.
			firstPage = false
		} else {
			sampleCount := float64(pageHeader.GranulePosition - lastGranule)
			if sampleCount > 0 {
				sampleDuration = time.Duration((sampleCount/48000.0)*1000) * time.Millisecond
			}
		}
		lastGranule = pageHeader.GranulePosition

		b.broadcastSample(media.Sample{
			Data:     pageData,
			Duration: sampleDuration,
		})
	}
}
