package main

import (
	"EasyConnectSuite/engine"
	"golang.org/x/sys/windows"
)

func (a *App) cleanupTUNOnStartup() (bool, error) { return engine.CleanupStaleTUN(true) }

type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Window     windows.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   windows.Handle
	IDList     uintptr
	Class      *uint16
	ClassKey   windows.Handle
	HotKey     uint32
	Icon       windows.Handle
	Process    windows.Handle
}
