package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
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

type testRTPWriter struct {
	packets [][]byte
}

func (w *testRTPWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	w.packets = append(w.packets, append([]byte(nil), payload...))
	return len(payload), nil
}

func (w *testRTPWriter) Write(b []byte) (int, error) {
	w.packets = append(w.packets, append([]byte(nil), b...))
	return len(b), nil
}

type testTrackContext struct {
	writer webrtc.TrackLocalWriter
}

func (c *testTrackContext) CodecParameters() []webrtc.RTPCodecParameters {
	return []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			},
			PayloadType: 96,
		},
	}
}
func (c *testTrackContext) HeaderExtensions() []webrtc.RTPHeaderExtensionParameter { return nil }
func (c *testTrackContext) SSRC() webrtc.SSRC                                      { return 1234 }
func (c *testTrackContext) SSRCRetransmission() webrtc.SSRC                        { return 0 }
func (c *testTrackContext) SSRCForwardErrorCorrection() webrtc.SSRC                { return 0 }
func (c *testTrackContext) WriteStream() webrtc.TrackLocalWriter                   { return c.writer }
func (c *testTrackContext) ID() string                                             { return "test-track" }
func (c *testTrackContext) RTCPReader() interceptor.RTCPReader                     { return nil }

func TestSPSPPSCachingAndImmediateDelivery(t *testing.T) {
	expectedSPS := []byte{0x67, 0x42, 0xe0, 0x1f, 0xda, 0x01, 0x40, 0x16}
	expectedPPS := []byte{0x68, 0xce, 0x3c, 0x80}

	b := &ScreenBroadcaster{
		tracks:  make(map[*webrtc.TrackLocalStaticSample]struct{}),
		running: true, // simulate running broadcaster
		sps:     expectedSPS,
		pps:     expectedPPS,
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
		t.Fatalf("Failed to create track: %v", err)
	}

	mockWriter := &testRTPWriter{}
	ctx := &testTrackContext{writer: mockWriter}
	params, err := track.Bind(ctx)
	t.Logf("Bind returned: params=%+v, err=%v", params, err)
	if err != nil {
		t.Fatalf("Failed to bind track to test context: %v", err)
	}

	// RegisterTrack writes the cached SPS and PPS to the track's H264 payloader.
	b.RegisterTrack(track)

	b.mu.Lock()
	count := len(b.tracks)
	b.mu.Unlock()

	if count != 1 {
		t.Fatalf("Expected 1 track registered, got %d", count)
	}

	// In RFC 6184, Pion's H264Payloader caches SPS and PPS, and on the next frame (IDR or non-IDR)
	// packs them together into a STAP-A (type 24) RTP packet.
	// Verify that sending an IDR frame produces a STAP-A packet containing the cached SPS and PPS.
	idrSlice := []byte{0x65, 0x88, 0x84, 0x00, 0x10}
	if err := track.WriteSample(media.Sample{Data: idrSlice, Duration: 16 * time.Millisecond}); err != nil {
		t.Fatalf("WriteSample IDR failed: %v", err)
	}

	if len(mockWriter.packets) == 0 {
		t.Fatalf("Expected RTP packet containing STAP-A, got 0 packets")
	}

	// STAP-A packet has NALU type 24 (0x78)
	firstPacket := mockWriter.packets[0]
	nalType := firstPacket[0] & 0x1F
	if nalType != 24 { // stapaNALUType = 24
		t.Fatalf("Expected STAP-A packet (type 24) containing SPS/PPS, got type %d", nalType)
	}

	// Parse STAP-A to verify it contains the SPS and PPS
	var h264Packet codecs.H264Packet
	annexB, err := h264Packet.Unmarshal(firstPacket)
	if err != nil {
		t.Fatalf("Failed to unmarshal STAP-A payload: %v", err)
	}

	reader, err := h264reader.NewReader(bytes.NewReader(annexB))
	if err != nil {
		t.Fatalf("Failed to parse Annex-B from STAP-A: %v", err)
	}

	var nalTypes []h264reader.NalUnitType
	for {
		nal, err := reader.NextNAL()
		if err != nil {
			break
		}
		nalTypes = append(nalTypes, nal.UnitType)
	}

	if len(nalTypes) < 2 || nalTypes[0] != h264reader.NalUnitTypeSPS || nalTypes[1] != h264reader.NalUnitTypePPS {
		t.Fatalf("Expected STAP-A to contain SPS and PPS, got NAL types: %v", nalTypes)
	}

	t.Logf("Successfully verified STAP-A RTP packet with cached SPS/PPS delivered: NAL types %v", nalTypes)
}

func TestEncoderPeriodicHeaderFlags(t *testing.T) {
	ffmpegPath := getFFmpegPath()
	config := detectBestVideoEncoder(ffmpegPath)

	hasFlag := func(flag string) bool {
		for _, arg := range config.Args {
			if arg == flag {
				return true
			}
		}
		return false
	}

	switch config.Encoder {
	case "h264_amf":
		if !hasFlag("-header_spacing") || !hasFlag("-forced_idr") {
			t.Fatalf("h264_amf missing -header_spacing or -forced_idr: %v", config.Args)
		}
	case "h264_nvenc":
		if !hasFlag("-forced-idr") || !hasFlag("-repeat-headers") {
			t.Fatalf("h264_nvenc missing -forced-idr or -repeat-headers: %v", config.Args)
		}
	case "libx264":
		if !hasFlag("-x264-params") {
			t.Fatalf("libx264 missing -x264-params: %v", config.Args)
		}
	}
	t.Logf("Encoder %s has correct periodic header flags in args: %v", config.Encoder, config.Args)
}

func TestScreenBroadcasterLifecycle(t *testing.T) {
	b := &ScreenBroadcaster{
		tracks:  make(map[*webrtc.TrackLocalStaticSample]struct{}),
		running: true,
		sps:     []byte{0x67, 0x42, 0xe0, 0x1f},
		pps:     []byte{0x68, 0xce, 0x3c, 0x80},
	}

	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"desktop-stream",
	)
	if err != nil {
		t.Fatalf("Failed to create track: %v", err)
	}

	b.RegisterTrack(track)
	if b.GetActiveClientsCount() != 1 {
		t.Fatalf("Expected 1 active client, got %d", b.GetActiveClientsCount())
	}

	// Unregister track: starts idle timer
	b.UnregisterTrack(track)
	if b.GetActiveClientsCount() != 0 {
		t.Fatalf("Expected 0 active clients, got %d", b.GetActiveClientsCount())
	}

	b.mu.Lock()
	if b.idleTimer == nil {
		b.mu.Unlock()
		t.Fatalf("Expected idleTimer to be started after unregistering all tracks")
	}
	b.mu.Unlock()

	// Registering a new track should stop the idle timer
	track2, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video",
		"desktop-stream",
	)
	if err != nil {
		t.Fatalf("Failed to create second track: %v", err)
	}

	b.RegisterTrack(track2)
	b.mu.Lock()
	if b.idleTimer != nil {
		b.mu.Unlock()
		t.Fatalf("Expected idleTimer to be cancelled upon new track registration")
	}
	b.mu.Unlock()

	// Explicit Stop should clear state
	b.Stop()
	b.mu.Lock()
	if b.running || len(b.tracks) != 0 || b.sps != nil || b.pps != nil {
		b.mu.Unlock()
		t.Fatalf("Stop() did not clear broadcaster state: running=%v, tracks=%d, sps=%v, pps=%v",
			b.running, len(b.tracks), b.sps, b.pps)
	}
	b.mu.Unlock()
	t.Logf("ScreenBroadcaster lifecycle test passed")
}

