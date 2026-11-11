package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	vpnlog "EasyConnectSuite/vpncore/log"
	"github.com/wailsapp/wails/v2/pkg/options"
)

func setStdErrHandle(*os.File)   {}
func showStartupError(err error) { _, _ = os.Stderr.WriteString(err.Error() + "\n") }
func configurePlatformOptions(app *options.App, _ *App) {
	app.HideWindowOnClose = false
	if app.OnStartup != nil && app.OnShutdown != nil {
		startup, shutdown := app.OnStartup, app.OnShutdown
		startupDone := make(chan struct{})
		app.OnStartup = func(ctx context.Context) {
			defer close(startupDone)
			startup(ctx)
		}
		app.OnShutdown = func(ctx context.Context) {
			op := vpnlog.BeginOperation(fmt.Sprintf("[VPN pid=%d] ", os.Getpid()), "linux_shutdown", nil)
			op.Step("wait_startup")
			<-startupDone
			op.Finish("startup_completed", nil)
			shutdown(ctx)
		}
	}
	app.OnBeforeClose = func(context.Context) bool {
		waitForLinuxGTKMainLoop()
		return false
	}
}
func platformCloseToTray() bool { return false }
func applicationStorageRoot() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config")
	}
	return ""
}
func configureDefaultProfile(*Profile) {}
