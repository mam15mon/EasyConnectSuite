//go:build windows

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	winsys "golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func setStdErrHandle(f *os.File) {
	_ = winsys.SetStdHandle(winsys.STD_ERROR_HANDLE, winsys.Handle(f.Fd()))
}

func applicationStorageRoot() string   { return os.Getenv("APPDATA") }
func configureDefaultProfile(*Profile) {}

func showStartupError(err error) {
	message, _ := winsys.UTF16PtrFromString(err.Error())
	title, _ := winsys.UTF16PtrFromString("VPN 启动失败")
	_, _ = winsys.MessageBox(0, message, title, winsys.MB_OK|winsys.MB_ICONERROR)
}

func configurePlatformOptions(app *options.App) {
	app.Windows = &windows.Options{
		WebviewUserDataPath:  filepath.Join(applicationDataDir(), "webview"),
		WebviewIsTransparent: false,
		WindowIsTranslucent:  false,
		DisableWindowIcon:    false,
	}
}
