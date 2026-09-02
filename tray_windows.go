//go:build windows
package main

import (
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procShell_NotifyIconW   = shell32.NewProc("Shell_NotifyIconW")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
)

const (
	WM_USER            = 0x0400
	WM_TRAYICON        = WM_USER + 1
	WM_COMMAND         = 0x0111
	WM_RBUTTONUP       = 0x0205
	WM_LBUTTONDBLCLK   = 0x0203
	NIM_ADD            = 0
	NIM_MODIFY         = 1
	NIM_DELETE         = 2
	NIF_MESSAGE        = 1
	NIF_ICON           = 2
	NIF_TIP            = 4
	NIF_INFO           = 0x10
	NIIF_INFO          = 1
	MF_STRING          = 0
	MF_CHECKED         = 8
	MF_UNCHECKED       = 0
	MF_SEPARATOR       = 0x0800
	TPM_RIGHTBUTTON    = 2
	IDI_APPLICATION    = 32512
	CMD_COPY_URL       = 1001
	CMD_OPEN_BROWSER   = 1002
	CMD_AUTOSTART      = 1003
	CMD_EXIT           = 1004

	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002
)

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	TimeoutOrVersion uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

var (
	trayMu         sync.Mutex
	currentURL     string
	trayHWnd       uintptr
	onExitCallback func()
)

func copyToClipboard(text string) {
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return
	}
	size := len(utf16) * 2

	hMem, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(size))
	if hMem == 0 {
		return
	}
	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return
	}
	for i, val := range utf16 {
		*(*uint16)(unsafe.Pointer(ptr + uintptr(i*2))) = val
	}
	procGlobalUnlock.Call(hMem)

	for attempts := 0; attempts < 10; attempts++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r != 0 {
			procEmptyClipboard.Call()
			procSetClipboardData.Call(CF_UNICODETEXT, hMem)
			procCloseClipboard.Call()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func openBrowser(url string) {
	if url == "" {
		url = "http://localhost:8080"
	}
	_ = exec.Command("cmd", "/c", "start", "", url).Start()
}

func showTrayMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	trayMu.Lock()
	url := currentURL
	trayMu.Unlock()

	label := "Скопировать ссылку"
	if url == "" {
		label = "Ожидание туннеля (Localhost)"
	}
	pCopy, _ := syscall.UTF16PtrFromString(label)
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(CMD_COPY_URL), uintptr(unsafe.Pointer(pCopy)))

	openLabel := "Открыть в браузере"
	if url == "" {
		openLabel = "Открыть в браузере (Localhost)"
	}
	pOpen, _ := syscall.UTF16PtrFromString(openLabel)
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(CMD_OPEN_BROWSER), uintptr(unsafe.Pointer(pOpen)))

	procAppendMenuW.Call(hMenu, uintptr(MF_SEPARATOR), 0, 0)

	autoFlags := uintptr(MF_STRING | MF_UNCHECKED)
	if isAutostartEnabled() {
		autoFlags = uintptr(MF_STRING | MF_CHECKED)
	}
	pAuto, _ := syscall.UTF16PtrFromString("Автозапуск с Windows")
	procAppendMenuW.Call(hMenu, autoFlags, uintptr(CMD_AUTOSTART), uintptr(unsafe.Pointer(pAuto)))

	procAppendMenuW.Call(hMenu, uintptr(MF_SEPARATOR), 0, 0)

	pExit, _ := syscall.UTF16PtrFromString("Выход")
	procAppendMenuW.Call(hMenu, uintptr(MF_STRING), uintptr(CMD_EXIT), uintptr(unsafe.Pointer(pExit)))

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(hwnd)
	procTrackPopupMenu.Call(hMenu, uintptr(TPM_RIGHTBUTTON), uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAYICON:
		if lParam == WM_RBUTTONUP {
			showTrayMenu(hwnd)
		} else if lParam == WM_LBUTTONDBLCLK {
			trayMu.Lock()
			url := currentURL
			trayMu.Unlock()
			openBrowser(url)
		}
	case WM_COMMAND:
		switch int(wParam & 0xFFFF) {
		case CMD_COPY_URL:
			trayMu.Lock()
			url := currentURL
			trayMu.Unlock()
			if url != "" {
				copyToClipboard(url)
			} else {
				copyToClipboard("http://localhost:8080")
			}
		case CMD_OPEN_BROWSER:
			trayMu.Lock()
			url := currentURL
			trayMu.Unlock()
			openBrowser(url)
		case CMD_AUTOSTART:
			_ = setAutostart(!isAutostartEnabled())
		case CMD_EXIT:
			removeTrayIcon()
			if onExitCallback != nil {
				onExitCallback()
			}
			procPostQuitMessage.Call(0)
		}
	default:
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return r
	}
	return 0
}

func initTray(onExit func()) {
	runtime.LockOSThread()
	onExitCallback = onExit
	className, _ := syscall.UTF16PtrFromString("DeskRemoteTrayClass")
	hIcon, _, _ := procLoadIconW.Call(0, uintptr(IDI_APPLICATION))

	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(wndProc)
	wc.LpszClassName = className
	wc.HIcon = hIcon

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	trayHWnd = hwnd

	var nid NOTIFYICONDATAW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_TRAYICON
	nid.HIcon = hIcon
	tip, _ := syscall.UTF16FromString("DeskRemote: Запуск...")
	copy(nid.SzTip[:], tip)

	procShell_NotifyIconW.Call(uintptr(NIM_ADD), uintptr(unsafe.Pointer(&nid)))

	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func notifyURLReady(url string) {
	trayMu.Lock()
	currentURL = url
	trayMu.Unlock()

	copyToClipboard(url)
	if trayHWnd == 0 {
		return
	}

	var nid NOTIFYICONDATAW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayHWnd
	nid.UID = 1
	nid.UFlags = NIF_TIP

	tip, _ := syscall.UTF16FromString("DeskRemote: Активен")
	copy(nid.SzTip[:], tip)

	procShell_NotifyIconW.Call(uintptr(NIM_MODIFY), uintptr(unsafe.Pointer(&nid)))
}

func removeTrayIcon() {
	if trayHWnd != 0 {
		var nid NOTIFYICONDATAW
		nid.CbSize = uint32(unsafe.Sizeof(nid))
		nid.HWnd = trayHWnd
		nid.UID = 1
		procShell_NotifyIconW.Call(uintptr(NIM_DELETE), uintptr(unsafe.Pointer(&nid)))
	}
}
