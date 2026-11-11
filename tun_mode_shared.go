//go:build windows || darwin || linux

package main

import (
	"EasyConnectSuite/engine"
	"context"
	"errors"
	"fmt"
)

type tunControl interface {
	engine.TUNController
	Prepare(context.Context, string) error
	Release(context.Context, string) error
	Close()
}
type tunCapable interface {
	SetTUN(context.Context, bool) error
	TUNState() engine.TUNStatus
}

func (a *App) tunControllerLocked() tunControl {
	if a.tunManager == nil {
		a.tunManager = newTUNManager()
	}
	return a.tunManager
}
func (a *App) SetTUNMode(enabled bool) error {
	a.mu.Lock()
	id := a.appConfig.ActiveProfileID
	a.mu.Unlock()
	return a.SetProfileTUNMode(id, enabled)
}
func (a *App) SetProfileTUNMode(id string, enabled bool) error {
	a.mu.Lock()
	manager := a.tunControllerLocked()
	a.mu.Unlock()
	return a.setProfileTUNMode(id, enabled, manager)
}
func (a *App) setProfileTUNMode(id string, enabled bool, manager tunControl) error {
	a.mu.Lock()
	c := a.connections[id]
	if a.starting || a.shuttingDown || a.modeOps[id] || (c != nil && (c.stopping || (c.status.Running && (c.status.State != "connected" && (enabled || c.status.State != "connecting" || c.status.Stage != "重连中"))))) {
		a.mu.Unlock()
		return errors.New("该配置正在连接、断开或切换 TUN，请稍候")
	}
	index := -1
	for i, p := range a.appConfig.Profiles {
		if p.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		a.mu.Unlock()
		return errors.New("服务器配置不存在")
	}
	previous := a.appConfig.Profiles[index].TUNMode
	a.appConfig.Profiles[index].TUNMode = enabled
	if err := a.saveConfigFile(); err != nil {
		a.appConfig.Profiles[index].TUNMode = previous
		a.mu.Unlock()
		return fmt.Errorf("保存 TUN 配置失败：%w", err)
	}
	if a.modeOps == nil {
		a.modeOps = map[string]bool{}
	}
	a.modeOps[id] = true
	running := c != nil && c.status.Running
	a.mu.Unlock()
	var err error
	if running {
		capability, ok := c.engine.(tunCapable)
		if !ok {
			err = errors.New("该 VPN 引擎不支持 TUN 热附着")
		} else {
			err = capability.SetTUN(c.ctx, enabled)
		}
	} else if enabled {
		err = manager.Prepare(context.Background(), id)
	} else {
		err = manager.Release(context.Background(), id)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.modeOps, id)
	if err != nil {
		for i := range a.appConfig.Profiles {
			if a.appConfig.Profiles[i].ID == id {
				a.appConfig.Profiles[i].TUNMode = previous
				if saveErr := a.saveConfigFile(); saveErr != nil {
					err = errors.Join(err, fmt.Errorf("还原 TUN 配置失败：%w", saveErr))
				}
				break
			}
		}
	}
	a.selectConnectionLocked()
	return err
}
