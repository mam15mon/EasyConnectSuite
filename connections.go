package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"EasyConnectSuite/engine"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type vpnEngine interface {
	Running() bool
	Stop()
	Resources() []engine.ResourceEntry
}
type vpnConnection struct {
	ctx       context.Context
	engine    vpnEngine
	cancel    context.CancelFunc
	ready     chan struct{}
	status    VPNStatus
	resources []engine.ResourceEntry
	started   time.Time
	slot      int
	stopDone  chan struct{}
	stopping  bool
}

func (a *App) normalizeProfiles() {
	for i := range a.appConfig.Profiles {
		p := &a.appConfig.Profiles[i]
		if p.Protocol == "" {
			p.Protocol = "easyconnect"
		}
		if p.SocksPort == 0 {
			p.SocksPort = 1080 + i*2
		}
		if p.HTTPPort == 0 {
			p.HTTPPort = 1081 + i*2
		}
	}
}
func (a *App) assignProxyPorts(p *Profile) {
	if p.SocksPort != 0 && p.HTTPPort != 0 {
		return
	}
	for port := 1080; port < 65535; port += 2 {
		used := false
		for _, existing := range a.appConfig.Profiles {
			if existing.ID == p.ID {
				continue
			}
			if existing.SocksPort == port || existing.HTTPPort == port || existing.SocksPort == port+1 || existing.HTTPPort == port+1 {
				used = true
				break
			}
		}
		if !used {
			if p.SocksPort == 0 {
				p.SocksPort = port
			}
			if p.HTTPPort == 0 {
				p.HTTPPort = port + 1
			}
			return
		}
	}
}
func disconnectedStatus(p Profile) VPNStatus {
	return VPNStatus{ProfileID: p.ID, Protocol: p.Protocol, ActiveProfile: p.Name, State: "disconnected", TUNMode: p.TUNMode, DisableProxy: p.DisableProxy, SocksPort: fmt.Sprint(p.SocksPort), HTTPPort: fmt.Sprint(p.HTTPPort)}
}
func (a *App) selectConnectionLocked() {
	if c := a.connections[a.appConfig.ActiveProfileID]; c != nil {
		a.status = c.status
		a.resources = c.resources
	} else {
		a.status = disconnectedStatus(a.getActiveProfileLocked())
		a.resources = nil
	}
	a.emitConnectionLocked(a.status, a.resources)
}
func (a *App) emitConnectionLocked(status VPNStatus, resources []engine.ResourceEntry) {
	a.syncTrayTooltipLocked()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "vpn-status", status)
		runtime.EventsEmit(a.ctx, "vpn-resources", map[string]any{"profileId": status.ProfileID, "entries": resources})
		runtime.EventsEmit(a.ctx, "vpn-connections", a.connectionStatusesLocked())
	}
}
func (a *App) connectionStatusesLocked() []VPNStatus {
	statuses := make([]VPNStatus, 0, len(a.appConfig.Profiles))
	for _, p := range a.appConfig.Profiles {
		if c := a.connections[p.ID]; c != nil {
			statuses = append(statuses, c.status)
		} else {
			statuses = append(statuses, disconnectedStatus(p))
		}
	}
	return statuses
}
func (a *App) GetConnections() []VPNStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connectionStatusesLocked()
}
func (a *App) anyRunningLocked() bool {
	for _, c := range a.connections {
		if c.status.Running {
			return true
		}
	}
	return false
}
func (a *App) connectionStatus(id string, c *vpnConnection, s engine.Status) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.connections[id] != c || c.stopping || c.ctx.Err() != nil {
		return
	}
	applyEngineStatus(&c.status, &c.started, &c.resources, s)
	if id == a.appConfig.ActiveProfileID {
		a.status = c.status
		a.resources = c.resources
	}
	a.emitConnectionLocked(c.status, c.resources)
}
func (a *App) connectionResources(id string, c *vpnConnection, entries []engine.ResourceEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.connections[id] != c || c.stopping || c.ctx.Err() != nil {
		return
	}
	c.resources = append([]engine.ResourceEntry(nil), entries...)
	if id == a.appConfig.ActiveProfileID {
		a.resources = c.resources
	}
	a.emitConnectionLocked(c.status, c.resources)
}
func (a *App) StartVPN() string {
	return a.startConnection(func(opts engine.Options) (vpnEngine, error) { return engine.Start(opts) })
}
func (a *App) StartProfileVPN(id string) string {
	return a.startConnectionID(id, func(opts engine.Options) (vpnEngine, error) { return engine.Start(opts) })
}
func (a *App) startConnection(start func(engine.Options) (vpnEngine, error)) string {
	a.mu.Lock()
	id := a.appConfig.ActiveProfileID
	a.mu.Unlock()
	return a.startConnectionID(id, start)
}
func (a *App) startConnectionID(id string, start func(engine.Options) (vpnEngine, error)) string {
	a.mu.Lock()
	if a.shuttingDown {
		a.mu.Unlock()
		return "程序正在退出"
	}
	if a.restarting {
		a.mu.Unlock()
		return "正在以管理员身份重新启动"
	}
	if a.starting {
		a.mu.Unlock()
		return "正在初始化或重启，请稍候"
	}
	var p Profile
	for _, profile := range a.appConfig.Profiles {
		if profile.ID == id {
			p = profile
			break
		}
	}
	if p.ID == "" {
		a.mu.Unlock()
		return "服务器配置不存在"
	}
	if p.Server == "" || p.Username == "" {
		a.mu.Unlock()
		return "配置不完整：服务器地址或用户名为空"
	}
	if p.Protocol == "univpn" && p.Enable2FA {
		a.mu.Unlock()
		return "UniVPN 当前仅支持账号密码认证"
	}
	if previous := a.connections[p.ID]; previous != nil && (previous.status.Running || previous.stopping) {
		a.mu.Unlock()
		return "该配置正在连接或断开"
	}
	slot := 0
	for ; slot < 64; slot++ {
		used := false
		for _, other := range a.connections {
			if other.status.Running || other.stopping {
				if other.slot == slot {
					used = true
					break
				}
			}
		}
		if !used {
			break
		}
	}
	if slot == 64 {
		a.mu.Unlock()
		return "最多支持 64 条同时连接"
	}
	if a.connections == nil {
		a.connections = make(map[string]*vpnConnection)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &vpnConnection{ctx: ctx, cancel: cancel, ready: make(chan struct{}), started: time.Now(), slot: slot, status: disconnectedStatus(p)}
	c.status.Running = true
	c.status.State = "connecting"
	c.status.Stage = "准备中"
	c.status.Message = "正在准备连接…"
	a.connections[p.ID] = c
	if p.ID == a.appConfig.ActiveProfileID {
		a.status = c.status
	}
	a.emitConnectionLocked(c.status, nil)
	logDir := filepath.Join(os.Getenv("APPDATA"), "EasyConnectVPN", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		delete(a.connections, p.ID)
		cancel()
		a.status = disconnectedStatus(p)
		a.emitConnectionLocked(a.status, nil)
		a.mu.Unlock()
		return err.Error()
	}
	opts := engine.Options{Config: a.buildEngineConfig(p), Context: ctx, Domain: p.Domain, TUNSlot: slot, LogFile: filepath.Join(logDir, "engine.log"),
		OnStatus: func(s engine.Status) { a.connectionStatus(p.ID, c, s) }, OnResources: func(rows []engine.ResourceEntry) { a.connectionResources(p.ID, c, rows) }}
	a.mu.Unlock()
	eng, err := start(opts)
	a.mu.Lock()
	c.engine = eng
	if !c.stopping {
		if err != nil {
			applyEngineStatus(&c.status, &c.started, &c.resources, engine.Status{Phase: engine.PhaseError, Message: err.Error()})
			cancel()
		} else {
			c.resources = eng.Resources()
		}
		if p.ID == a.appConfig.ActiveProfileID {
			a.status = c.status
			a.resources = c.resources
		}
		a.emitConnectionLocked(c.status, c.resources)
	}
	close(c.ready)
	a.mu.Unlock()
	if err != nil {
		return err.Error()
	}
	return "OK"
}
func (a *App) stopConnection(id string) string {
	a.mu.Lock()
	c := a.connections[id]
	if c == nil {
		a.mu.Unlock()
		return "Not running"
	}
	if c.stopping {
		done := c.stopDone
		a.mu.Unlock()
		<-done
		return "OK"
	}
	c.stopping = true
	c.stopDone = make(chan struct{})
	c.cancel()
	a.mu.Unlock()
	<-c.ready
	if c.engine != nil {
		c.engine.Stop()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	c.stopping = false
	close(c.stopDone)
	applyEngineStatus(&c.status, &c.started, &c.resources, engine.Status{Phase: engine.PhaseDisconnected, Message: "VPN 已断开"})
	if id == a.appConfig.ActiveProfileID {
		a.status = c.status
		a.resources = nil
	}
	a.emitConnectionLocked(c.status, nil)
	return "OK"
}
func (a *App) StopProfileVPN(id string) string { return a.stopConnection(id) }
func (a *App) StopVPN() string {
	a.mu.Lock()
	id := a.appConfig.ActiveProfileID
	a.mu.Unlock()
	return a.stopConnection(id)
}
func (a *App) StopAllVPN() string {
	a.mu.Lock()
	ids := make([]string, 0, len(a.connections))
	for id := range a.connections {
		ids = append(ids, id)
	}
	a.mu.Unlock()
	for _, id := range ids {
		a.stopConnection(id)
	}
	return "OK"
}
