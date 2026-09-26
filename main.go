package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	winsys "golang.org/x/sys/windows"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// setupCrashLog redirects the process stderr to a persistent file. In a
// windowsgui app stderr is invalid by default, so a Go panic anywhere in
// any goroutine would otherwise kill the process silently and leave no
// trace. With this in place the runtime panic dump lands in
// %APPDATA%\EasyConnectVPN\logs\crash.log.
func setupCrashLog() {
	debug.SetTraceback("all")

	logDir := ""
	if appData := os.Getenv("APPDATA"); appData != "" {
		logDir = filepath.Join(appData, "EasyConnectVPN", "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return
		}
	} else {
		logDir = "."
	}

	path := filepath.Join(logDir, "crash.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 8*1024*1024 {
		_ = os.Rename(path, path+".old")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(fmt.Sprintf("\n=== application started %s ===\n", time.Now().Format(time.RFC3339)))
	_ = winsys.SetStdHandle(winsys.STD_ERROR_HANDLE, winsys.Handle(f.Fd()))
}

func main() {
	setupCrashLog()
	app := NewApp()

	err := wails.Run(&options.App{
		Title:             "EasyConnect VPN 管理器",
		Width:             980,
		Height:            700,
		MinWidth:          840,
		MinHeight:         600,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
