package main

import (
	"context"
	"fmt"
	"log"
	"os"
	goruntime "runtime"
	"syscall"
	"unsafe"

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
	globalTrayApp  *App
	globalTrayHwnd uintptr
	globalNID      NOTIFYICONDATA
)

func trayWndProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	if msg == WM_POWERBROADCAST && globalTrayApp != nil {
		if globalTrayApp.logPowerEvent(wParam) {
			return 1
		}
	}
	if msg == WM_TRAYICON {
		switch lParam {
		case WM_LBUTTONUP, WM_LBUTTONDBLCLK:
			if globalTrayApp != nil && globalTrayApp.ctx != nil {
				wailsruntime.WindowShow(globalTrayApp.ctx)
				wailsruntime.WindowUnminimise(globalTrayApp.ctx)
			}
			return 0
		case WM_RBUTTONUP:
			var pt POINT
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			procSetForegroundWindow.Call(hwnd)

			if globalTrayApp == nil {
				return 0
			}

			status := globalTrayApp.GetStatus()
			cfg := globalTrayApp.GetAppConfig()
			activeProfile := globalTrayApp.GetActiveProfile()

			hMenu, _, _ := procCreatePopupMenu.Call()

			// 1. Status Header Line (Grayed out informational line)
			var statusText string
			if status.Running {
				if status.State == "connected" {
					if status.ClientIP != "" {
						statusText = fmt.Sprintf("● 已连接: %s (%s)", status.ActiveProfile, status.ClientIP)
					} else {
						statusText = fmt.Sprintf("● 已连接: %s", status.ActiveProfile)
					}
				} else {
					statusText = fmt.Sprintf("⏳ 握手中: %s...", status.ActiveProfile)
				}
			} else {
				statusText = "○ 状态: 未连接 (空闲就绪)"
			}
			sPtr, _ := syscall.UTF16PtrFromString(statusText)
			procAppendMenuW.Call(hMenu, MF_STRING|MF_GRAYED|MF_DISABLED, 1000, uintptr(unsafe.Pointer(sPtr)))
			procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

			// 2. Direct Profile Switch Submenu: ⚡ 切换并连接配置 ▶
			hSubMenu, _, _ := procCreatePopupMenu.Call()
			for i, p := range cfg.Profiles {
				cmdID := uintptr(2000 + i)
				pLabel := p.Name
				if p.Enable2FA {
					pLabel += " [2FA]"
				}
				var flags uintptr = MF_STRING
				if p.ID == cfg.ActiveProfileID {
					if status.Running {
						flags |= MF_CHECKED
						pLabel = "✓ " + pLabel + " (当前连接)"
					} else {
						pLabel = "★ " + pLabel + " (当前默认)"
					}
				}
				pPtr, _ := syscall.UTF16PtrFromString(pLabel)
				procAppendMenuW.Call(hSubMenu, flags, cmdID, uintptr(unsafe.Pointer(pPtr)))
			}
			subTitlePtr, _ := syscall.UTF16PtrFromString("⚡ 切换并连接配置")
			procAppendMenuW.Call(hMenu, MF_POPUP, hSubMenu, uintptr(unsafe.Pointer(subTitlePtr)))

			procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

			// 3. Connect Action: Enabled when NOT running, Grayed when already running
			connectLabel := fmt.Sprintf("▶ 连接当前配置 (%s)", activeProfile.Name)
			connPtr, _ := syscall.UTF16PtrFromString(connectLabel)
			var connFlags uintptr = MF_STRING
			if status.Running {
				connFlags |= MF_GRAYED | MF_DISABLED
			}
			procAppendMenuW.Call(hMenu, connFlags, 1001, uintptr(unsafe.Pointer(connPtr)))

			// 4. Disconnect Action: Enabled when running, Grayed when NOT running
			discPtr, _ := syscall.UTF16PtrFromString("⏹ 断开 VPN 连接")
			var discFlags uintptr = MF_STRING
			if !status.Running {
				discFlags |= MF_GRAYED | MF_DISABLED
			}
			procAppendMenuW.Call(hMenu, discFlags, 1002, uintptr(unsafe.Pointer(discPtr)))

			procAppendMenuW.Call(hMenu, MF_SEPARATOR, 0, 0)

			// 5. Show Main Window
			showPtr, _ := syscall.UTF16PtrFromString("💻 显示主控制台")
			procAppendMenuW.Call(hMenu, MF_STRING, 1003, uintptr(unsafe.Pointer(showPtr)))

			// 6. Exit Application
			exitPtr, _ := syscall.UTF16PtrFromString("✕ 退出程序")
			procAppendMenuW.Call(hMenu, MF_STRING, 1004, uintptr(unsafe.Pointer(exitPtr)))

			cmd, _, _ := procTrackPopupMenu.Call(
				hMenu,
				TPM_RIGHTBUTTON|TPM_RETURNCMD,
				uintptr(pt.X),
				uintptr(pt.Y),
				0,
				hwnd,
				0,
			)
			procDestroyMenu.Call(hMenu)

			switch {
			case cmd == 1001: // Connect current
				go globalTrayApp.StartVPN()
			case cmd == 1002: // Disconnect
				go globalTrayApp.StopVPN()
			case cmd == 1003: // Show Window
				if globalTrayApp.ctx != nil {
					wailsruntime.WindowShow(globalTrayApp.ctx)
					wailsruntime.WindowUnminimise(globalTrayApp.ctx)
				}
			case cmd == 1004: // Exit
				RemoveTrayIcon()
				globalTrayApp.shutdown(globalTrayApp.ctx)
				if globalTrayApp.ctx != nil {
					wailsruntime.Quit(globalTrayApp.ctx)
				}
			case cmd >= 2000 && int(cmd-2000) < len(cfg.Profiles):
				targetProfile := cfg.Profiles[cmd-2000]
				go func() {
					globalTrayApp.SetActiveProfile(targetProfile.ID)
					globalTrayApp.StartVPN()
					if globalTrayApp.ctx != nil {
						wailsruntime.EventsEmit(globalTrayApp.ctx, "profile-switched", targetProfile.ID)
					}
				}()
			}
			return 0
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

// StartTray initializes Windows system tray icon
func StartTray(app *App, ctx context.Context) {
	globalTrayApp = app

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
		globalTrayHwnd = hwnd

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

		globalNID = NOTIFYICONDATA{
			cbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATA{})),
			hWnd:             hwnd,
			uID:              1,
			uFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
			uCallbackMessage: WM_TRAYICON,
			hIcon:            hIcon,
		}

		tipStr := "EasyConnect VPN (未连接)"
		tipRunes := []rune(tipStr)
		for i, r := range tipRunes {
			if i < 127 {
				globalNID.szTip[i] = uint16(r)
			}
		}

		procShell_NotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&globalNID)))

		var msg MSG
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}

		procShell_NotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&globalNID)))
	}()
}

// UpdateTrayTooltip modifies the tooltip string dynamically
func UpdateTrayTooltip(tip string) {
	if globalTrayHwnd == 0 || globalNID.hWnd == 0 {
		return
	}
	for i := range globalNID.szTip {
		globalNID.szTip[i] = 0
	}
	tipRunes := []rune(tip)
	for i, r := range tipRunes {
		if i < 127 {
			globalNID.szTip[i] = uint16(r)
		}
	}
	globalNID.uFlags = NIF_TIP
	procShell_NotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&globalNID)))
	globalNID.uFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
}

// RemoveTrayIcon removes the icon from system tray
func RemoveTrayIcon() {
	if globalNID.hWnd != 0 {
		procShell_NotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&globalNID)))
		globalNID.hWnd = 0
	}
	if globalTrayHwnd != 0 {
		procDestroyWindow.Call(globalTrayHwnd)
		procPostQuitMessage.Call(0)
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
