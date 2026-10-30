//go:build !windows

package main

import (
	"EasyConnectSuite/engine"
	"context"
	"errors"
)

type tunControl interface {
	engine.TUNController
	Prepare(context.Context, string) error
	Release(context.Context, string) error
	Close()
}

type unsupportedTUNManager struct{}

func (unsupportedTUNManager) Attach(context.Context, engine.TUNSpec, engine.TUNBackend) (engine.TUNAttachment, error) {
	return nil, errors.New("macOS TUN 尚未实现")
}
func (unsupportedTUNManager) Prepare(context.Context, string) error {
	return errors.New("macOS TUN 尚未实现")
}
func (unsupportedTUNManager) Release(context.Context, string) error {
	return errors.New("macOS TUN 尚未实现")
}
func (unsupportedTUNManager) Close() {}

func newTUNManager() tunControl { return unsupportedTUNManager{} }
func (a *App) tunControllerLocked() tunControl {
	if a.tunManager == nil {
		a.tunManager = newTUNManager()
	}
	return a.tunManager
}
func (a *App) cleanupTUNOnStartup() (bool, error) { return false, nil }

func (a *App) SetTUNMode(enabled bool) error {
	a.mu.Lock()
	id := a.appConfig.ActiveProfileID
	a.mu.Unlock()
	return a.SetProfileTUNMode(id, enabled)
}

func (a *App) SetProfileTUNMode(id string, enabled bool) error {
	return errors.New("macOS TUN 尚未实现")
}

func (a *App) setProfileTUNMode(id string, enabled bool, manager tunControl) error {
	if enabled {
		return manager.Prepare(context.Background(), id)
	}
	return manager.Release(context.Background(), id)
}
