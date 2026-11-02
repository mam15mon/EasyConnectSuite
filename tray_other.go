//go:build !windows && !darwin

package main

import "context"

func StartTray(_ *App, _ context.Context) {}
func RemoveTrayIcon()                     {}
func showTrayWorkspace(_ *App)            {}
func UpdateTrayTooltip(string)            {}
