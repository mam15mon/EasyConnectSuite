//go:build windows

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func setStdErrHandle(f *os.File) {
	_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
}

func showStartupError(err error) {
	message, _ := windows.UTF16PtrFromString(err.Error())
	title, _ := windows.UTF16PtrFromString("VPN 启动失败")
	_, _ = windows.MessageBox(0, message, title, windows.MB_OK|windows.MB_ICONERROR)
}

func configurePlatformOptions(app *options.App) {
	app.Windows = &windows.Options{
		WebviewUserDataPath:  filepath.Join(applicationDataDir(), "webview"),
		WebviewIsTransparent: false,
		WindowIsTranslucent:  false,
		DisableWindowIcon:    false,
	}
}
