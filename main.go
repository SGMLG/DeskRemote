package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

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

func main() {
	if exePath, err := os.Executable(); err == nil {
		_ = os.Chdir(filepath.Dir(exePath))
	}

	tokenFlag := flag.String("token", "", "Cloudflare Tunnel Token")
	pinFlag := flag.String("pin", "1234", "PIN-код для доступа к экрану")
	flag.Parse()

	screenW, screenH := getScreenResolution()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		clientPin := r.URL.Query().Get("pin")
		if *pinFlag != "" && clientPin != *pinFlag {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		handleSignaling(w, r, screenW, screenH)
	})

	ctx, cancel := context.WithCancel(context.Background())
	server := &http.Server{Addr: ":8080"}

	go func() {
		_ = server.ListenAndServe()
	}()

	go startCloudflareTunnel(ctx, *tokenFlag, func(url string) {
		notifyURLReady(url)
	})

	initTray(func() {
		cancel()
		_ = server.Shutdown(context.Background())
	})
}

func handleSignaling(w http.ResponseWriter, r *http.Request, screenW, screenH int) {
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

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video",
		"desktop-stream",
	)
	if err != nil {
		logToFile(fmt.Sprintf("[WebRTC] Ошибка NewTrackLocalStaticSample: %v", err))
		return
	}

	rtpSender, err := pc.AddTrack(videoTrack)
	if err != nil {
		logToFile(fmt.Sprintf("[WebRTC] Ошибка AddTrack: %v", err))
		return
	}

	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, err := rtpSender.Read(buf); err != nil {
				return
			}
		}
	}()

	var (
		captureCancel context.CancelFunc
		captureMu     sync.Mutex
	)

	stopCapture := func() {
		captureMu.Lock()
		defer captureMu.Unlock()
		if captureCancel != nil {
			captureCancel()
			captureCancel = nil
		}
	}
	defer stopCapture()

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logToFile(fmt.Sprintf("[WebRTC] Состояние соединения: %s", state.String()))
		if state == webrtc.PeerConnectionStateConnected {
			captureMu.Lock()
			if captureCancel != nil {
				captureCancel()
			}
			var capCtx context.Context
			capCtx, captureCancel = context.WithCancel(context.Background())
			go startScreenCapture(capCtx, videoTrack)
			captureMu.Unlock()
		} else if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateDisconnected {
			stopCapture()
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
				handleInputEvent(inputMsg, screenW, screenH)
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
