//go:build !windows

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"os"
)

func setStdErrHandle(f *os.File)                {}
func showStartupError(err error)                { _, _ = os.Stderr.WriteString(err.Error() + "\n") }
func configurePlatformOptions(app *options.App) {}
func runTUNWorker([]string) error               { return os.ErrInvalid }
