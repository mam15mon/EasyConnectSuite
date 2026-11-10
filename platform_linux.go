package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func setStdErrHandle(*os.File)   {}
func showStartupError(err error) { _, _ = os.Stderr.WriteString(err.Error() + "\n") }
func configurePlatformOptions(app *options.App, _ *App) {
	app.HideWindowOnClose = false
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
func configureDefaultProfile(p *Profile) { p.TUNMode = false; p.DisableProxy = false }
