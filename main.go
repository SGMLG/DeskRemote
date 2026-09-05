package main

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

//go:embed index.html
var indexHTML []byte

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type SignalMessage struct {
	Type      string                  `json:"type"`
	SDP       string                  `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
}

type PinRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]int
	lockouts map[string]time.Time
}

var rateLimiter = &PinRateLimiter{
	attempts: make(map[string]int),
	lockouts: make(map[string]time.Time),
}

func init() {
	// Background cleanup of expired RateLimiter entries to prevent memory leak
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rateLimiter.mu.Lock()
			now := time.Now()
			for ip, lockUntil := range rateLimiter.lockouts {
				if now.After(lockUntil) {
					delete(rateLimiter.lockouts, ip)
					delete(rateLimiter.attempts, ip)
				}
			}
			rateLimiter.mu.Unlock()
		}
	}()
}

func getClientIP(r *http.Request) string {
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		return strings.TrimSpace(cfIP)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (limiter *PinRateLimiter) CheckAndVerify(r *http.Request, providedPin, expectedPin string) (bool, string) {
	ip := getClientIP(r)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()
	if lockUntil, locked := limiter.lockouts[ip]; locked {
		if now.Before(lockUntil) {
			remaining := int(time.Until(lockUntil).Seconds())
			return false, fmt.Sprintf("Слишком много неверных попыток. IP заблокирован на %d сек.", remaining)
		}
		delete(limiter.lockouts, ip)
		delete(limiter.attempts, ip)
	}

	valid := expectedPin == "" || subtle.ConstantTimeCompare([]byte(providedPin), []byte(expectedPin)) == 1
	if !valid {
		limiter.attempts[ip]++
		count := limiter.attempts[ip]
		if count >= 5 {
			limiter.lockouts[ip] = now.Add(5 * time.Minute)
			logToFile(fmt.Sprintf("[Security] IP %s заблокирован на 5 минут после %d неудачных попыток ввода PIN", ip, count))
			return false, "Слишком много неверных попыток. Доступ заблокирован на 5 минут."
		}
		logToFile(fmt.Sprintf("[Security] Неверный PIN от IP %s (попытка %d/5)", ip, count))
		return false, "Неверный PIN-код"
	}

	delete(limiter.attempts, ip)
	return true, ""
}

func main() {
	if exePath, err := os.Executable(); err == nil {
		_ = os.Chdir(filepath.Dir(exePath))
	}

	// Enable DPI awareness for accurate multi-monitor and scaled coordinates
	if procSetProcessDPIAware != nil {
		_, _, _ = procSetProcessDPIAware.Call()
	}

	checkDependencies()

	tokenFlag := flag.String("token", "", "Cloudflare Tunnel Token")
	pinFlag := flag.String("pin", "1234", "PIN-код для доступа к экрану")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Write(indexHTML)
	})

	http.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write([]byte(`{
  "name": "DeskRemote - Удаленное управление",
  "short_name": "DeskRemote",
  "description": "Быстрый пульт дистанционного управления ПК через WebRTC",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#070b14",
  "theme_color": "#070b14",
  "orientation": "any",
  "icons": [
    {
      "src": "/icon.svg",
      "sizes": "any",
      "type": "image/svg+xml",
      "purpose": "any maskable"
    }
  ]
}`))
	})

	http.HandleFunc("/icon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
  <defs>
    <linearGradient id="bg" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#0b1329"/>
      <stop offset="100%" stop-color="#141e33"/>
    </linearGradient>
    <linearGradient id="screenGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" stop-color="#0e1726"/>
      <stop offset="100%" stop-color="#050a12"/>
    </linearGradient>
  </defs>
  <rect width="512" height="512" rx="112" fill="url(#bg)"/>
  <rect x="72" y="96" width="368" height="236" rx="20" fill="url(#screenGrad)" stroke="#38bdf8" stroke-width="8"/>
  <path d="M190 214 L256 160 L322 214" fill="none" stroke="#38bdf8" stroke-width="12" stroke-linecap="round" stroke-linejoin="round" opacity="0.4"/>
  <path d="M210 236 L256 198 L302 236" fill="none" stroke="#38bdf8" stroke-width="10" stroke-linecap="round" stroke-linejoin="round" opacity="0.7"/>
  <polygon points="256,230 256,280 270,266 286,276 292,266 276,256 290,256" fill="#38bdf8"/>
  <path d="M232 332 L224 396 L288 396 L280 332 Z" fill="#1e293b"/>
  <rect x="180" y="396" width="152" height="18" rx="9" fill="#334155"/>
  <circle cx="256" cy="122" r="6" fill="#22c55e"/>
</svg>`))
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		clientPin := r.URL.Query().Get("pin")
		ok, errMsg := rateLimiter.CheckAndVerify(r, clientPin, *pinFlag)
		if !ok {
			http.Error(w, errMsg, http.StatusForbidden)
			return
		}
		handleSignaling(w, r)
	})

	ctx, cancel := context.WithCancel(context.Background())
	server := &http.Server{Addr: ":8080"}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logToFile(fmt.Sprintf("[HTTP] Критическая ошибка ListenAndServe: %v", err))
		}
	}()

	go startCloudflareTunnel(ctx, *tokenFlag, func(url string) {
		notifyURLReady(url)
	})

	logToFile("[Main] Сервер запущен на :8080, инициализация системного трея...")
	initTray(func() {
		cancel()
		broadcaster.Stop()
		audioBroadcaster.Stop()
		_ = server.Shutdown(context.Background())
	})
}

func handleSignaling(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	var wsMutex sync.Mutex
	sendWS := func(msg SignalMessage) {
		wsMutex.Lock()
		defer wsMutex.Unlock()
		_ = ws.WriteJSON(msg)
	}

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
				"stun:stun2.l.google.com:19302",
				"stun:stun.cloudflare.com:3478",
			}},
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		logToFile(fmt.Sprintf("[WebRTC] Ошибка NewPeerConnection: %v", err))
		return
	}
	defer pc.Close()

	// High-performance H.264 video track (Constrained Baseline 3.1)
	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		"video",
		"desktop-stream",
	)
	if err != nil {
		logToFile(fmt.Sprintf("[WebRTC] Ошибка NewTrackLocalStaticSample (video): %v", err))
		return
	}

	rtpVideoSender, err := pc.AddTrack(videoTrack)
	if err != nil {
		logToFile(fmt.Sprintf("[WebRTC] Ошибка AddTrack (video): %v", err))
		return
	}

	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := rtpVideoSender.Read(buf); err != nil {
				return
			}
		}
	}()

	// Low-latency Opus stereo audio track
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"desktop-stream",
	)
	if err != nil {
		logToFile(fmt.Sprintf("[WebRTC] Ошибка NewTrackLocalStaticSample (audio): %v", err))
		return
	}

	rtpAudioSender, err := pc.AddTrack(audioTrack)
	if err != nil {
		logToFile(fmt.Sprintf("[WebRTC] Ошибка AddTrack (audio): %v", err))
		return
	}

	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := rtpAudioSender.Read(buf); err != nil {
				return
			}
		}
	}()

	var unregOnce sync.Once
	unregisterAll := func() {
		unregOnce.Do(func() {
			broadcaster.UnregisterTrack(videoTrack)
			audioBroadcaster.UnregisterTrack(audioTrack)
		})
	}
	defer unregisterAll()

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logToFile(fmt.Sprintf("[WebRTC] Состояние соединения: %s", state.String()))
		if state == webrtc.PeerConnectionStateConnected {
			broadcaster.RegisterTrack(videoTrack)
			audioBroadcaster.RegisterTrack(audioTrack)
		} else if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateDisconnected {
			unregisterAll()
		}
	})

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		c := candidate.ToJSON()
		sendWS(SignalMessage{Type: "candidate", Candidate: &c})
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "input" {
			return
		}
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			var inputMsg InputMessage
			if err := json.Unmarshal(msg.Data, &inputMsg); err == nil {
				handleInputEvent(inputMsg)
			}
		})
	})

	for {
		_, rawMsg, err := ws.ReadMessage()
		if err != nil {
			break
		}

		var msg SignalMessage
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "offer":
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: msg.SDP}); err != nil {
				logToFile(fmt.Sprintf("[WebRTC] SetRemoteDescription error: %v", err))
				return
			}
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				logToFile(fmt.Sprintf("[WebRTC] CreateAnswer error: %v", err))
				return
			}
			if err := pc.SetLocalDescription(answer); err != nil {
				logToFile(fmt.Sprintf("[WebRTC] SetLocalDescription error: %v", err))
				return
			}
			sendWS(SignalMessage{Type: "answer", SDP: answer.SDP})
		case "candidate":
			if msg.Candidate != nil {
				_ = pc.AddICECandidate(*msg.Candidate)
			}
		}
	}
}
