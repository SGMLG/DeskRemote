//go:build windows
package main

import (
	"strings"
	"syscall"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procMouseEvent       = user32.NewProc("mouse_event")
	procKeybdEvent       = user32.NewProc("keybd_event")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	SM_CXSCREEN          = 0
	SM_CYSCREEN          = 1
	MOUSEEVENTF_LEFTDOWN = 0x0002
	MOUSEEVENTF_LEFTUP   = 0x0004
	MOUSEEVENTF_RIGHTDOWN = 0x0008
	MOUSEEVENTF_RIGHTUP   = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL     = 0x0800

	KEYEVENTF_KEYUP   = 0x0002
	KEYEVENTF_UNICODE = 0x0004
)

type InputMessage struct {
	Type   string  `json:"t"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Button int     `json:"b,omitempty"`
	DeltaY int     `json:"dy,omitempty"`
	Key    string  `json:"key,omitempty"`
	Code   string  `json:"code,omitempty"`
	Str    string  `json:"str,omitempty"`
}

var specialKeyMap = map[string]uintptr{
	"Backspace":    0x08,
	"Tab":          0x09,
	"Enter":        0x0D,
	"Shift":        0x10,
	"ShiftLeft":    0x10,
	"ShiftRight":   0x10,
	"Control":      0x11,
	"ControlLeft":  0x11,
	"ControlRight": 0x11,
	"Alt":          0x12,
	"AltLeft":      0x12,
	"AltRight":     0x12,
	"Pause":        0x13,
	"CapsLock":     0x14,
	"Escape":       0x1B,
	"Space":        0x20,
	" ":            0x20,
	"PageUp":       0x21,
	"PageDown":     0x22,
	"End":          0x23,
	"Home":         0x24,
	"ArrowLeft":    0x25,
	"ArrowUp":      0x26,
	"ArrowRight":   0x27,
	"ArrowDown":    0x28,
	"PrintScreen":  0x2C,
	"Insert":       0x2D,
	"Delete":       0x2E,
	"Meta":         0x5B, // Windows Key
	"MetaLeft":     0x5B,
	"MetaRight":    0x5C,
	"F1":           0x70,
	"F2":           0x71,
	"F3":           0x72,
	"F4":           0x73,
	"F5":           0x74,
	"F6":           0x75,
	"F7":           0x76,
	"F8":           0x77,
	"F9":           0x78,
	"F10":          0x79,
	"F11":          0x7A,
	"F12":          0x7B,
}

func getVK(key, code string) uintptr {
	if vk, ok := specialKeyMap[code]; ok {
		return vk
	}
	if vk, ok := specialKeyMap[key]; ok {
		return vk
	}
	if strings.HasPrefix(code, "Key") && len(code) == 4 {
		ch := code[3]
		if ch >= 'A' && ch <= 'Z' {
			return uintptr(ch)
		}
	}
	if strings.HasPrefix(code, "Digit") && len(code) == 6 {
		ch := code[5]
		if ch >= '0' && ch <= '9' {
			return uintptr(ch)
		}
	}
	if len(key) == 1 {
		ch := key[0]
		if ch >= 'a' && ch <= 'z' {
			return uintptr(ch - 32)
		}
		if ch >= 'A' && ch <= 'Z' {
			return uintptr(ch)
		}
	}
	return 0
}

func getScreenResolution() (int, int) {
	w, _, _ := procGetSystemMetrics.Call(uintptr(SM_CXSCREEN))
	h, _, _ := procGetSystemMetrics.Call(uintptr(SM_CYSCREEN))
	return int(w), int(h)
}

func handleInputEvent(msg InputMessage, screenW, screenH int) {
	switch msg.Type {
	case "mm":
		targetX := int(msg.X * float64(screenW))
		targetY := int(msg.Y * float64(screenH))
		procSetCursorPos.Call(uintptr(targetX), uintptr(targetY))
	case "md":
		var flag uintptr
		switch msg.Button {
		case 0:
			flag = MOUSEEVENTF_LEFTDOWN
		case 1:
			flag = MOUSEEVENTF_MIDDLEDOWN
		case 2:
			flag = MOUSEEVENTF_RIGHTDOWN
		}
		if flag != 0 {
			procMouseEvent.Call(flag, 0, 0, 0, 0)
		}
	case "mu":
		var flag uintptr
		switch msg.Button {
		case 0:
			flag = MOUSEEVENTF_LEFTUP
		case 1:
			flag = MOUSEEVENTF_MIDDLEUP
		case 2:
			flag = MOUSEEVENTF_RIGHTUP
		}
		if flag != 0 {
			procMouseEvent.Call(flag, 0, 0, 0, 0)
		}
	case "mw":
		procMouseEvent.Call(MOUSEEVENTF_WHEEL, 0, 0, uintptr(uint32(msg.DeltaY*120)), 0)
	case "kd":
		vk := getVK(msg.Key, msg.Code)
		if vk != 0 {
			procKeybdEvent.Call(vk, 0, 0, 0)
		}
	case "ku":
		vk := getVK(msg.Key, msg.Code)
		if vk != 0 {
			procKeybdEvent.Call(vk, 0, KEYEVENTF_KEYUP, 0)
		}
	case "char":
		for _, r := range []rune(msg.Str) {
			procKeybdEvent.Call(0, uintptr(r), KEYEVENTF_UNICODE, 0)
			procKeybdEvent.Call(0, uintptr(r), KEYEVENTF_UNICODE|KEYEVENTF_KEYUP, 0)
		}
	}
}
