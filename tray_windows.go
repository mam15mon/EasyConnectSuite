package main

import (
	"context"
	"fmt"
	"log"
	"os"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	vpnlog "EasyConnectSuite/vpncore/log"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	NIM_ADD           = 0x00000000
	NIM_MODIFY        = 0x00000001
	NIM_DELETE        = 0x00000002
	NIF_MESSAGE       = 0x00000001
	NIF_ICON          = 0x00000002
	NIF_TIP           = 0x00000004
	WM_POWERBROADCAST = 0x0218
	WM_USER           = 0x0400
	WM_TRAYICON       = WM_USER + 101
	WM_TRAYQUIT       = WM_USER + 102
	WM_LBUTTONUP      = 0x0202
	WM_RBUTTONUP      = 0x0205
	WM_LBUTTONDBLCLK  = 0x0203
	IDI_APPLICATION   = 32512
	MF_STRING         = 0x00000000
	MF_GRAYED         = 0x00000001
	MF_DISABLED       = 0x00000002
	MF_CHECKED        = 0x00000008
	MF_POPUP          = 0x00000010
	MF_SEPARATOR      = 0x00000800
	TPM_RIGHTBUTTON   = 0x0002
	TPM_RETURNCMD     = 0x0100
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procPostMessageW        = user32.NewProc("PostMessageW")

	procShell_NotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconExW    = shell32.NewProc("ExtractIconExW")
	procGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
)

type WNDCLASSEX struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

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

type NOTIFYICONDATA struct {
	cbSize            uint32
	hWnd              uintptr
	uID               uint32
	uFlags            uint32
	uCallbackMessage  uint32
	hIcon             uintptr
	szTip             [128]uint16
	dwState           uint32
	dwStateMask       uint32
	szInfo            [256]uint16
	uTimeoutOrVersion uint32
	szInfoTitle       [64]uint16
	dwInfoFlags       uint32
	guidItem          [16]byte
	hBalloonIcon      uintptr
}

var (
	globalTrayApp     *App
	globalTrayHwnd    uintptr
	globalNID         NOTIFYICONDATA
	trayIconMu        sync.Mutex
	trayClosed        bool
	showTrayWindow    = wailsruntime.WindowShow
	restoreTrayWindow = wailsruntime.WindowUnminimise
)

func trayWndProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	if msg == WM_TRAYQUIT {
		procDestroyWindow.Call(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	}
	if msg == WM_POWERBROADCAST && globalTrayApp != nil {
		if globalTrayApp.logPowerEvent(wParam) {
			return 1
		}
	}
	if msg == WM_TRAYICON {
		switch lParam {
		case WM_LBUTTONUP, WM_LBUTTONDBLCLK:
			if globalTrayApp != nil {
				showTrayWorkspace(globalTrayApp)
			}
			return 0
		case WM_RBUTTONUP:
			var pt POINT
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			procSetForegroundWindow.Call(hwnd)

			if globalTrayApp == nil {
				return 0
			}

			showTrayMenu(globalTrayApp, hwnd, pt)
			return 0
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func appendTrayMenu(menu, flags, id uintptr, text string) {
	// Ampersands in profile names are text, not native menu accelerators.
	ptr, _ := syscall.UTF16PtrFromString(strings.ReplaceAll(text, "&", "&&"))
	procAppendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(ptr)))
}

func buildTrayMenu(statuses []VPNStatus) (uintptr, map[uintptr]trayAction) {
	menu, _, _ := procCreatePopupMenu.Call()
	appendTrayMenu(menu, MF_GRAYED, 0, traySummary(statuses))
	appendTrayMenu(menu, MF_STRING, 1003, "打开 VPN 工作台")
	procAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	actions := make(map[uintptr]trayAction, len(statuses)*2)
	var running bool
	for i, s := range statuses {
		submenu, _, _ := procCreatePopupMenu.Call()
		protocol := s.Protocol
		switch protocol {
		case "easyconnect":
			protocol = "EasyConnect"
		case "univpn":
			protocol = "UniVPN"
		case "topsec":
			protocol = "TopSAP"
		}
		flags := uintptr(MF_POPUP)
		if s.Running && s.State == "connected" {
			flags |= MF_CHECKED
		}
		appendTrayMenu(menu, flags, submenu, fmt.Sprintf("%s · %s · %s", s.ActiveProfile, protocol, trayState(s)))
		label, action := trayLifecycleAction(s)
		id := uintptr(2000 + i*2)
		actions[id] = action
		flags = MF_STRING
		if s.State == "stopping" {
			flags |= MF_GRAYED
		}
		appendTrayMenu(submenu, flags, id, label)
		actions[id+1] = trayAction{kind: "details", profileID: s.ProfileID}
		appendTrayMenu(submenu, MF_STRING, id+1, "查看详情")
		running = running || s.Running
	}
	if len(statuses) == 0 {
		appendTrayMenu(menu, MF_GRAYED, 0, "暂无 VPN 配置，请打开工作台添加")
	}
	procAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	flags := uintptr(MF_STRING)
	if !running {
		flags |= MF_GRAYED
	}
	appendTrayMenu(menu, flags, 1002, "断开全部 VPN")
	appendTrayMenu(menu, MF_STRING, 1004, "退出程序（断开全部）")
	return menu, actions
}

func showTrayMenu(app *App, hwnd uintptr, pt POINT) {
	menu, actions := buildTrayMenu(app.trayStatuses())
	defer procDestroyMenu.Call(menu)
	cmd, _, _ := procTrackPopupMenu.Call(menu, TPM_RIGHTBUTTON|TPM_RETURNCMD, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	switch cmd {
	case 1003:
		showTrayWorkspace(app)
	case 1004:
		go func() {
			app.mu.Lock()
			if app.shuttingDown {
				app.mu.Unlock()
				return
			}
			app.shuttingDown = true
			app.mu.Unlock()
			log.Printf("[VPN pid=%d] tray action=quit", os.Getpid())
			if app.ctx != nil {
				wailsruntime.Quit(app.ctx)
			}
		}()
	case 1002:
		go app.executeTrayAction(trayAction{kind: "disconnect_all"})
	default:
		if action, ok := actions[cmd]; ok {
			if action.kind == "details" {
				if app.SetActiveProfile(action.profileID) {
					if app.ctx != nil {
						wailsruntime.EventsEmit(app.ctx, "profile-switched", action.profileID)
					}
					showTrayWorkspace(app)
				}
			} else {
				go func() {
					result, cancelled := app.executeTrayAction(action)
					if result != "OK" && !cancelled && app.ctx != nil {
						_, _ = wailsruntime.MessageDialog(app.ctx, wailsruntime.MessageDialogOptions{Type: wailsruntime.ErrorDialog, Title: "VPN 操作失败", Message: result})
					}
				}()
			}
		}
	}
}

func showTrayWorkspace(app *App) {
	diagnostic := vpnlog.BeginOperation(fmt.Sprintf("[VPN pid=%d] ", os.Getpid()), "tray_workspace", nil)
	outcome := "skipped"
	defer func() { diagnostic.Finish(outcome, nil) }()
	diagnostic.Step("wait_app_lock")
	app.mu.Lock()
	ctx, shuttingDown := app.ctx, app.shuttingDown
	app.mu.Unlock()
	if ctx == nil || shuttingDown {
		return
	}
	// Window restoration can synchronously reenter trayWndProc on this thread.
	// Never hold the application lock across either native window operation.
	diagnostic.Step("window_show")
	showTrayWindow(ctx)
	diagnostic.Step("window_restore")
	restoreTrayWindow(ctx)
	outcome = "shown"
}

// StartTray initializes Windows system tray icon
func StartTray(app *App, ctx context.Context) {
	globalTrayApp = app
	trayIconMu.Lock()
	trayClosed = false
	trayIconMu.Unlock()

	go func() {
		goruntime.LockOSThread()
		defer goruntime.UnlockOSThread()

		hInstance, _, _ := procGetModuleHandleW.Call(0)
		className, _ := syscall.UTF16PtrFromString("EasyConnectTrayClass")

		wcex := WNDCLASSEX{
			cbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
			lpfnWndProc:   syscall.NewCallback(trayWndProc),
			hInstance:     hInstance,
			lpszClassName: className,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcex)))

		windowName, _ := syscall.UTF16PtrFromString("EasyConnectTrayWindow")
		hwnd, _, _ := procCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(windowName)),
			0,
			0, 0, 0, 0,
			0, 0, hInstance, 0,
		)

		// Extract custom high-resolution icon from the compiled executable
		exePath, _ := os.Executable()
		var hIconLarge, hIconSmall uintptr
		procExtractIconExW.Call(
			uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(exePath))),
			0,
			uintptr(unsafe.Pointer(&hIconLarge)),
			uintptr(unsafe.Pointer(&hIconSmall)),
			1,
		)

		hIcon := hIconSmall
		if hIcon == 0 {
			hIcon = hIconLarge
		}
		if hIcon == 0 {
			hIcon, _, _ = procLoadIconW.Call(0, uintptr(IDI_APPLICATION))
		}

		tip := trayTooltip(app.trayStatuses())
		trayIconMu.Lock()
		if trayClosed {
			trayIconMu.Unlock()
			procDestroyWindow.Call(hwnd)
			return
		}
		globalTrayHwnd = hwnd
		globalNID = NOTIFYICONDATA{
			cbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
			hWnd:             hwnd,
			uID:              1,
			uFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
			uCallbackMessage: WM_TRAYICON,
			hIcon:            hIcon,
			szTip:            trayTooltipBuffer(tip),
		}

		procShell_NotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&globalNID)))
		trayIconMu.Unlock()

		var msg MSG
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}

		RemoveTrayIcon()
	}()
}

// UpdateTrayTooltip modifies the tooltip string dynamically
func UpdateTrayTooltip(tip string) {
	trayIconMu.Lock()
	defer trayIconMu.Unlock()
	if globalTrayHwnd == 0 || globalNID.hWnd == 0 {
		return
	}
	globalNID.szTip = trayTooltipBuffer(tip)
	globalNID.uFlags = NIF_TIP
	procShell_NotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&globalNID)))
	globalNID.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
}

// RemoveTrayIcon removes the icon from system tray
func RemoveTrayIcon() {
	trayIconMu.Lock()
	defer trayIconMu.Unlock()
	trayClosed = true
	if globalNID.hWnd != 0 {
		procShell_NotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&globalNID)))
		globalNID.hWnd = 0
	}
	if globalTrayHwnd != 0 {
		procPostMessageW.Call(globalTrayHwnd, WM_TRAYQUIT, 0, 0)
		globalTrayHwnd = 0
	}
}

// Windows supplies these events; they do not identify sleep versus hibernation.
func (a *App) logPowerEvent(event uintptr) bool {
	var name string
	switch event {
	case 4:
		name = "suspend"
	case 18:
		name = "resume_automatic"
	case 7:
		name = "resume_user"
	default:
		return false
	}
	log.Printf("System power pid=%d source=WM_POWERBROADCAST event=%s", os.Getpid(), name)
	for _, status := range a.GetConnections() {
		if status.Running {
			log.Printf("[VPN pid=%d profile=%q] power event=%s state=%s uptime_seconds=%d", os.Getpid(), status.ProfileID, name, status.State, status.Duration)
		}
	}
	return true
}
