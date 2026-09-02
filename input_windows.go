//go:build windows
package main

import "syscall"

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procMouseEvent       = user32.NewProc("mouse_event")
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
)

type InputMessage struct {
	Type   string  `json:"t"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Button int     `json:"b,omitempty"`
	DeltaY int     `json:"dy,omitempty"`
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
	}
}
