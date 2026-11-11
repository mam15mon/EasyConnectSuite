//go:build !windows && !darwin && !linux

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
	return nil, errors.New("当前平台 TUN 尚未完成")
}
func (unsupportedTUNManager) Prepare(context.Context, string) error {
	return errors.New("当前平台 TUN 尚未完成")
}
func (unsupportedTUNManager) Release(context.Context, string) error {
	return nil
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
	if enabled {
		return errors.New("当前平台 TUN 尚未完成")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.starting || a.shuttingDown || a.modeOps[id] {
		return errors.New("该配置正在连接、断开或切换 TUN，请稍候")
	}
	if c := a.connections[id]; c != nil && (c.stopping || c.status.Running) {
		return errors.New("请先断开该配置再修改 TUN")
	}
	for i := range a.appConfig.Profiles {
		if a.appConfig.Profiles[i].ID == id {
			previous := a.appConfig.Profiles[i].TUNMode
			a.appConfig.Profiles[i].TUNMode = false
			if err := a.saveConfigFile(); err != nil {
				a.appConfig.Profiles[i].TUNMode = previous
				return err
			}
			a.selectConnectionLocked()
			return nil
		}
	}
	return errors.New("服务器配置不存在")
}

func (a *App) setProfileTUNMode(id string, enabled bool, manager tunControl) error {
	if enabled {
		return manager.Prepare(context.Background(), id)
	}
	return manager.Release(context.Background(), id)
}
