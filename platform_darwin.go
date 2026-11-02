package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var hideDarwinWindow = wailsruntime.WindowHide

func setStdErrHandle(*os.File)   {}
func showStartupError(err error) { _, _ = os.Stderr.WriteString(err.Error() + "\n") }
func applicationStorageRoot() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
}
func configureDefaultProfile(*Profile) {}
func platformCloseToTray() bool        { return darwinTrayReady() }

func configurePlatformOptions(options *options.App, app *App) {
	options.HideWindowOnClose = false
	options.OnBeforeClose = func(ctx context.Context) bool {
		app.mu.Lock()
		shuttingDown := app.shuttingDown
		app.mu.Unlock()
		if shuttingDown || !darwinTrayReady() {
			return false
		}
		hideDarwinWindow(ctx)
		return true
	}
}
