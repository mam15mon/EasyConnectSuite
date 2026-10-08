package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"unsafe"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

// SetTUNMode returns true when an elevated replacement has been launched.
func (a *App) SetTUNMode(enabled bool) (bool, error) {
	restarting, err := a.setTUNMode(enabled, windows.GetCurrentProcessToken().IsElevated(), launchTUNAdministrator)
	if restarting {
		wailsRuntime.Quit(a.ctx)
	}
	return restarting, err
}

func (a *App) setTUNMode(enabled, elevated bool, launch func(string) error) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.starting || a.status.Running || a.restarting {
		return false, errors.New("请先断开连接，再切换 TUN 模式")
	}
	for i := range a.appConfig.Profiles {
		if a.appConfig.Profiles[i].ID != a.appConfig.ActiveProfileID {
			continue
		}
		previous := a.appConfig.Profiles[i].TUNMode
		a.appConfig.Profiles[i].TUNMode = enabled
		if err := a.saveConfigFile(); err != nil {
			a.appConfig.Profiles[i].TUNMode = previous
			return false, fmt.Errorf("保存 TUN 配置失败：%w", err)
		}
		if !enabled || elevated {
			return false, nil
		}
		if err := launch(a.configPath); err != nil {
			a.appConfig.Profiles[i].TUNMode = previous
			if saveErr := a.saveConfigFile(); saveErr != nil {
				return false, fmt.Errorf("%v；还原 TUN 配置失败：%w", err, saveErr)
			}
			return false, err
		}
		a.restarting = true
		return true, nil
	}
	return false, errors.New("请先选择服务器配置")
}

// SHELLEXECUTEINFOW layout from shellapi.h. NOASYNC waits for launch completion;
// NO_UI suppresses shell error dialogs, but keeps the Windows UAC prompt.
type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Window     windows.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   windows.Handle
	IDList     uintptr
	Class      *uint16
	ClassKey   windows.Handle
	HotKey     uint32
	Icon       windows.Handle
	Process    windows.Handle
}

func launchTUNAdministrator(configPath string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return err
	}
	parameters := windows.ComposeCommandLine([]string{"--tun-restart", strconv.Itoa(os.Getpid()), configPath})
	info := shellExecuteInfo{
		Mask:       0x100 | 0x400, // SEE_MASK_NOASYNC | SEE_MASK_FLAG_NO_UI
		Verb:       windows.StringToUTF16Ptr("runas"),
		File:       windows.StringToUTF16Ptr(executable),
		Parameters: windows.StringToUTF16Ptr(parameters),
		Directory:  windows.StringToUTF16Ptr(filepath.Dir(executable)),
		Show:       windows.SW_SHOWNORMAL,
	}
	info.Size = uint32(unsafe.Sizeof(info))
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ole := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole.NewProc("CoInitializeEx").Call(0, 2|4) // STA | DISABLE_OLE1DDE
	if int32(hr) < 0 {
		return fmt.Errorf("初始化管理员启动失败：0x%x", hr)
	}
	defer ole.NewProc("CoUninitialize").Call()
	result, _, callErr := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return errors.New("已取消管理员授权，TUN 模式保持原设置")
		}
		return fmt.Errorf("以管理员身份启动失败：%w", callErr)
	}
	return nil
}

// Wait before Wails acquires its single-instance mutex and reads the saved
// profile. Keeping the exact config path also supports portable installations.
func prepareTUNRestart(args []string) (string, error) {
	if len(args) == 0 || args[0] != "--tun-restart" {
		return "", nil
	}
	if len(args) != 3 {
		return "", errors.New("管理员重启参数不完整")
	}
	pid, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil || pid == 0 || pid == uint64(os.Getpid()) || !filepath.IsAbs(args[2]) {
		return "", errors.New("管理员重启参数无效")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return "", errors.New("管理员授权未生效，请重新开启 TUN 模式")
	}
	if err := waitForTUNParent(uint32(pid)); err != nil {
		return "", err
	}
	return args[2], nil
}

func waitForTUNParent(pid uint32) error {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil // The original process has already exited.
	}
	if err != nil {
		return fmt.Errorf("等待原窗口退出失败：%w", err)
	}
	defer windows.CloseHandle(process)
	result, err := windows.WaitForSingleObject(process, 15000)
	if err != nil {
		return err
	}
	if result != windows.WAIT_OBJECT_0 {
		return errors.New("原窗口未能退出，请关闭原程序后重新启动")
	}
	return nil
}

func showTUNRestartError(err error) {
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(err.Error()))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("EasyConnect TUN 启动失败"))), 0x10)
}
