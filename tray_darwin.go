package main

import (
	"fmt"
	"os"

	vpnlog "EasyConnectSuite/vpncore/log"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var showDarwinWindow = wailsruntime.WindowShow
var restoreDarwinWindow = wailsruntime.WindowUnminimise

func showTrayWorkspace(app *App) {
	op := vpnlog.BeginOperation(fmt.Sprintf("[VPN pid=%d] ", os.Getpid()), "darwin_workspace", nil)
	outcome := "skipped"
	defer func() { op.Finish(outcome, nil) }()
	op.Step("wait_app_lock")
	app.mu.Lock()
	ctx, shuttingDown := app.ctx, app.shuttingDown
	app.mu.Unlock()
	if ctx == nil || shuttingDown {
		return
	}
	op.Step("window_show")
	showDarwinWindow(ctx)
	op.Step("window_restore")
	restoreDarwinWindow(ctx)
	outcome = "shown"
}
