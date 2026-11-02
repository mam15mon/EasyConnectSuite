//go:build !windows

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"os"
	"path/filepath"
	"runtime"
)

func setStdErrHandle(f *os.File)                {}
func showStartupError(err error)                { _, _ = os.Stderr.WriteString(err.Error() + "\n") }
func configurePlatformOptions(app *options.App) { app.HideWindowOnClose = false }
func applicationStorageRoot() string {
	return filepath.Join(os.Getenv("HOME"), "Library", "Application Support")
}
func configureDefaultProfile(p *Profile) {
	if runtime.GOOS != "darwin" {
		p.TUNMode = false
		p.DisableProxy = false
	}
}
