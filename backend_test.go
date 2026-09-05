package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/h264reader"
)

func TestInputMessageUnmarshalFixed(t *testing.T) {
	// Test relative move
	dataMove := []byte(`{"t":"mrel","dx":15.5,"dy":-10.2}`)
	var msgMove InputMessage
	if err := json.Unmarshal(dataMove, &msgMove); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if msgMove.DX != 15.5 || msgMove.DY != -10.2 {
		t.Fatalf("Expected DX=15.5, DY=-10.2, got DX=%v, DY=%v", msgMove.DX, msgMove.DY)
	}

	// Test wheel
	dataWheel := []byte(`{"t":"mw","wheel":-1}`)
	var msgWheel InputMessage
	if err := json.Unmarshal(dataWheel, &msgWheel); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if msgWheel.Wheel != -1 {
		t.Fatalf("Expected Wheel=-1, got %v", msgWheel.Wheel)
	}
}

func TestPinRateLimiter(t *testing.T) {
	limiter := &PinRateLimiter{
		attempts: make(map[string]int),
		lockouts: make(map[string]time.Time),
	}

	req := httptest.NewRequest("GET", "/ws?pin=wrong", nil)
	req.RemoteAddr = "192.168.1.50:12345"

	// 4 wrong attempts
	for i := 1; i <= 4; i++ {
		ok, _ := limiter.CheckAndVerify(req, "wrong", "1234")
		if ok {
			t.Fatalf("Expected false on wrong PIN at attempt %d", i)
		}
	}

	// 5th wrong attempt should lock out
	ok, msg := limiter.CheckAndVerify(req, "wrong", "1234")
	if ok {
		t.Fatalf("Expected false on 5th wrong attempt")
	}
	if limiter.lockouts["192.168.1.50"].IsZero() {
		t.Fatalf("Expected IP to be locked out")
	}
	t.Logf("Lockout message: %s", msg)

	// Even correct PIN should now be rejected during lockout
	ok, _ = limiter.CheckAndVerify(req, "1234", "1234")
	if ok {
		t.Fatalf("Expected rejected during lockout period")
	}

	// Another IP should still be allowed
	req2 := httptest.NewRequest("GET", "/ws?pin=1234", nil)
	req2.RemoteAddr = "192.168.1.51:12345"
	ok2, _ := limiter.CheckAndVerify(req2, "1234", "1234")
	if !ok2 {
		t.Fatalf("Expected allowed for different IP")
	}
}

func TestInputStructSize(t *testing.T) {
	var in INPUT
	size := unsafe.Sizeof(in)
	t.Logf("sizeof(INPUT) = %d bytes", size)
	if size != 40 {
		t.Fatalf("Expected sizeof(INPUT) == 40 on 64-bit Windows, got %d", size)
	}
}

func TestH264ReaderAndTrack(t *testing.T) {
	// Sample H.264 Annex-B stream with start codes: SPS (0x67), PPS (0x68), IDR (0x65)
	stream := []byte{
		0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xe0, 0x1f, 0xda, 0x01, 0x40, 0x16, // SPS
		0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x3c, 0x80,                         // PPS
		0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00, 0x10,                   // IDR
	}

	reader, err := h264reader.NewReader(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		"video",
		"desktop-stream",
	)
	if err != nil {
		t.Fatalf("NewTrackLocalStaticSample failed: %v", err)
	}

	nalCount := 0
	for {
		nal, err := reader.NextNAL()
		if err != nil {
			break
		}
		nalCount++
		t.Logf("Read NAL unit type: %v, len: %d", nal.UnitType, len(nal.Data))
		// Writing to track without active peer won't fail
		_ = track.WriteSample(media.Sample{
			Data:     nal.Data,
			Duration: 16 * time.Millisecond,
		})
	}
	if nalCount != 3 {
		t.Fatalf("Expected 3 NAL units, got %d", nalCount)
	}
}

func TestDetectBestVideoEncoder(t *testing.T) {
	ffmpegPath := getFFmpegPath()
	config := detectBestVideoEncoder(ffmpegPath)
	t.Logf("Detected video encoder: %s (name: %s)", config.Encoder, config.Name)
	if config.Encoder == "" || len(config.Args) == 0 {
		t.Fatalf("Expected valid encoder config, got %+v", config)
	}
	if config.Encoder != "h264_amf" && config.Encoder != "h264_nvenc" && config.Encoder != "h264_qsv" && config.Encoder != "libx264" {
		t.Fatalf("Unexpected encoder: %s", config.Encoder)
	}
}

func TestOpusAudioTrack(t *testing.T) {
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"desktop-stream",
	)
	if err != nil {
		t.Fatalf("Failed to create Opus audio track: %v", err)
	}
	if audioTrack.Codec().MimeType != webrtc.MimeTypeOpus {
		t.Fatalf("Expected MimeTypeOpus, got %s", audioTrack.Codec().MimeType)
	}

	// Test sample write with 20ms Opus frame
	dummyOpusFrame := []byte{0xf8, 0xff, 0xfe} // small Opus TOC payload
	err = audioTrack.WriteSample(media.Sample{
		Data:     dummyOpusFrame,
		Duration: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("WriteSample failed: %v", err)
	}
}

func TestDetectedEncoderPipeline(t *testing.T) {
	ffmpegPath := getFFmpegPath()
	config := detectBestVideoEncoder(ffmpegPath)

	// Test full pipeline with test video source and detected encoder args
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=s=320x240:d=0.2:r=30",
	}
	args = append(args, config.Args...)

	cmd := exec.Command(ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe failed: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start failed: %v", err)
	}

	reader, err := h264reader.NewReader(stdout)
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	nalCount := 0
	for {
		nal, err := reader.NextNAL()
		if err != nil {
			break
		}
		nalCount++
		t.Logf("Read NAL unit #%d (type=%d, len=%d)", nalCount, nal.UnitType, len(nal.Data))
	}

	_ = cmd.Wait()
	if nalCount == 0 {
		t.Fatalf("Expected at least 1 NAL unit from encoder %s, got 0", config.Encoder)
	}
	t.Logf("Successfully read %d NAL units using encoder %s", nalCount, config.Encoder)
}

