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
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
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
	restartConfig, err := prepareTUNRestart(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		showTUNRestartError(err)
		return
	}
	app := NewApp()
	if restartConfig != "" {
		app.configPath = restartConfig
		app.initDefaultConfig()
		app.loadConfigFile()
		app.normalizeProfiles()
		app.selectConnectionLocked()
	}

	err = wails.Run(&options.App{
		Title:             "EasyConnect Suite · VPN 工作台",
		Width:             1080,
		Height:            820,
		MinWidth:          840,
		MinHeight:         660,
		HideWindowOnClose: true,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "EasyConnectSuite-6f1c2a90-INSTANCE",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				// 再次双击 exe 时,把已运行实例的窗口唤到前台,
				// 而不是悄悄再起一个连不上端口的实例
				if app.ctx != nil {
					wailsRuntime.WindowShow(app.ctx)
					wailsRuntime.WindowUnminimise(app.ctx)
				}
			},
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 244, G: 246, B: 250, A: 1},
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
