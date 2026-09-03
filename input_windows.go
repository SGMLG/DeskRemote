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

	KEYEVENTF_KEYUP = 0x0002
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

var keyMap = map[string]uintptr{
	// Row 1
	"Escape":    0x1B,
	"Digit1":    0x31, "1": 0x31,
	"Digit2":    0x32, "2": 0x32,
	"Digit3":    0x33, "3": 0x33,
	"Digit4":    0x34, "4": 0x34,
	"Digit5":    0x35, "5": 0x35,
	"Digit6":    0x36, "6": 0x36,
	"Digit7":    0x37, "7": 0x37,
	"Digit8":    0x38, "8": 0x38,
	"Digit9":    0x39, "9": 0x39,
	"Digit0":    0x30, "0": 0x30,
	"Minus":     0xBD, "-": 0xBD,
	"Equal":     0xBB, "=": 0xBB,
	"Backspace": 0x08,

	// Row 2
	"Tab":          0x09,
	"KeyQ":         0x51, "q": 0x51, "Q": 0x51,
	"KeyW":         0x57, "w": 0x57, "W": 0x57,
	"KeyE":         0x45, "e": 0x45, "E": 0x45,
	"KeyR":         0x52, "r": 0x52, "R": 0x52,
	"KeyT":         0x54, "t": 0x54, "T": 0x54,
	"KeyY":         0x59, "y": 0x59, "Y": 0x59,
	"KeyU":         0x55, "u": 0x55, "U": 0x55,
	"KeyI":         0x49, "i": 0x49, "I": 0x49,
	"KeyO":         0x4F, "o": 0x4F, "O": 0x4F,
	"KeyP":         0x50, "p": 0x50, "P": 0x50,
	"BracketLeft":  0xDB, "[": 0xDB,
	"BracketRight": 0xDD, "]": 0xDD,
	"Backslash":    0xDC, "\\": 0xDC,

	// Row 3
	"CapsLock":  0x14,
	"KeyA":      0x41, "a": 0x41, "A": 0x41,
	"KeyS":      0x53, "s": 0x53, "S": 0x53,
	"KeyD":      0x44, "d": 0x44, "D": 0x44,
	"KeyF":      0x46, "f": 0x46, "F": 0x46,
	"KeyG":      0x47, "g": 0x47, "G": 0x47,
	"KeyH":      0x48, "h": 0x48, "H": 0x48,
	"KeyJ":      0x4A, "j": 0x4A, "J": 0x4A,
	"KeyK":      0x4B, "k": 0x4B, "K": 0x4B,
	"KeyL":      0x4C, "l": 0x4C, "L": 0x4C,
	"Semicolon": 0xBA, ";": 0xBA,
	"Quote":     0xDE, "'": 0xDE,
	"Enter":     0x0D,

	// Row 4
	"ShiftLeft":   0x10,
	"ShiftRight":  0x10,
	"Shift":       0x10,
	"KeyZ":        0x5A, "z": 0x5A, "Z": 0x5A,
	"KeyX":        0x58, "x": 0x58, "X": 0x58,
	"KeyC":        0x43, "c": 0x43, "C": 0x43,
	"KeyV":        0x56, "v": 0x56, "V": 0x56,
	"KeyB":        0x42, "b": 0x42, "B": 0x42,
	"KeyN":        0x4E, "n": 0x4E, "N": 0x4E,
	"KeyM":        0x4D, "m": 0x4D, "M": 0x4D,
	"Comma":       0xBC, ",": 0xBC,
	"Period":      0xBE, ".": 0xBE,
	"Slash":       0xBF, "/": 0xBF,

	// Bottom row & Nav
	"ControlLeft":  0x11,
	"ControlRight": 0x11,
	"Control":      0x11,
	"AltLeft":      0x12,
	"AltRight":     0x12,
	"Alt":          0x12,
	"MetaLeft":     0x5B,
	"MetaRight":    0x5C,
	"Meta":         0x5B,
	"Space":        0x20, " ": 0x20,
	"ArrowLeft":    0x25,
	"ArrowUp":      0x26,
	"ArrowRight":   0x27,
	"ArrowDown":    0x28,
	"Delete":       0x2E,
	"Insert":       0x2D,
	"Home":         0x24,
	"End":          0x23,
	"PageUp":       0x21,
	"PageDown":     0x22,
	"PrintScreen":  0x2C,

	// Function Keys
	"F1": 0x70, "F2": 0x71, "F3": 0x72, "F4": 0x73,
	"F5": 0x74, "F6": 0x75, "F7": 0x76, "F8": 0x77,
	"F9": 0x78, "F10": 0x79, "F11": 0x7A, "F12": 0x7B,
}

var ruKeyMap = map[rune]uintptr{
	'й': 0x51, 'Й': 0x51,
	'ц': 0x57, 'Ц': 0x57,
	'у': 0x45, 'У': 0x45,
	'к': 0x52, 'К': 0x52,
	'е': 0x54, 'Е': 0x54,
	'н': 0x59, 'Н': 0x59,
	'г': 0x55, 'Г': 0x55,
	'ш': 0x49, 'Ш': 0x49,
	'щ': 0x4F, 'Щ': 0x4F,
	'з': 0x50, 'З': 0x50,
	'х': 0xDB, 'Х': 0xDB,
	'ъ': 0xDD, 'Ъ': 0xDD,
	'ф': 0x41, 'Ф': 0x41,
	'ы': 0x53, 'Ы': 0x53,
	'в': 0x44, 'В': 0x44,
	'а': 0x46, 'А': 0x46,
	'п': 0x47, 'П': 0x47,
	'р': 0x48, 'Р': 0x48,
	'о': 0x4A, 'О': 0x4A,
	'л': 0x4B, 'Л': 0x4B,
	'д': 0x4C, 'Д': 0x4C,
	'ж': 0xBA, 'Ж': 0xBA,
	'э': 0xDE, 'Э': 0xDE,
	'я': 0x5A, 'Я': 0x5A,
	'ч': 0x58, 'Ч': 0x58,
	'с': 0x43, 'С': 0x43,
	'м': 0x56, 'М': 0x56,
	'и': 0x42, 'И': 0x42,
	'т': 0x4E, 'Т': 0x4E,
	'ь': 0x4D, 'Ь': 0x4D,
	'б': 0xBC, 'Б': 0xBC,
	'ю': 0xBE, 'Ю': 0xBE,
	'ё': 0xC0, 'Ё': 0xC0,
}

func getVK(key, code string) uintptr {
	if vk, ok := keyMap[code]; ok {
		return vk
	}
	if vk, ok := keyMap[key]; ok {
		return vk
	}
	if len(key) > 0 {
		r := []rune(key)[0]
		if vk, ok := ruKeyMap[r]; ok {
			return vk
		}
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
	return 0
}

func pasteText(text string) {
	copyToClipboard(text)
	// Send Ctrl + V
	procKeybdEvent.Call(0x11, 0, 0, 0)
	procKeybdEvent.Call(0x56, 0, 0, 0)
	procKeybdEvent.Call(0x56, 0, KEYEVENTF_KEYUP, 0)
	procKeybdEvent.Call(0x11, 0, KEYEVENTF_KEYUP, 0)
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
		if len(msg.Str) > 0 {
			pasteText(msg.Str)
		}
	}
}
