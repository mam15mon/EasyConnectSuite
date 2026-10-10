package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"EasyConnectSuite/engine"

	"github.com/mythologyli/zju-connect/configs"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Profile represents a single VPN configuration
type Profile struct {
	ID               string `json:"id"`
	Protocol         string `json:"protocol"`
	Domain           string `json:"domain"`
	SocksPort        int    `json:"socksPort"`
	HTTPPort         int    `json:"httpPort"`
	Name             string `json:"name"`
	Server           string `json:"server"`
	Port             int    `json:"port"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	Enable2FA        bool   `json:"enable2FA"`
	TotpSecret       string `json:"totpSecret"`
	BindInterface    string `json:"bindInterface"`
	TUNMode          bool   `json:"tunMode"`
	DisableProxy     bool   `json:"disableProxy"`
	DisableZjuConfig bool   `json:"disableZjuConfig"`
	DisableKeepAlive bool   `json:"disableKeepAlive"`
	KeepAliveURL     string `json:"keepAliveUrl"`
}

// AppConfig stores all profiles and active selection
type AppConfig struct {
	ActiveProfileID string    `json:"activeProfileId"`
	Profiles        []Profile `json:"profiles"`
}

// VPNStatus contains current connection status
type VPNStatus struct {
	ProfileID     string `json:"profileId"`
	Protocol      string `json:"protocol"`
	Running       bool   `json:"running"`
	Stage         string `json:"stage"`
	Message       string `json:"message"`
	State         string `json:"state"` // "disconnected", "connecting", "connected", "error"
	ActiveProfile string `json:"activeProfile"`
	ClientIP      string `json:"clientIp"`
	DNS           string `json:"dns"`
	SocksPort     string `json:"socksPort"`
	HTTPPort      string `json:"httpPort"`
	TUNMode       bool   `json:"tunMode"`
	DisableProxy  bool   `json:"disableProxy"`
	Duration      int64  `json:"duration"` // uptime seconds
	LastError     string `json:"lastError"`
}

// App struct — the VPN engine now runs in-process (no embedded
// zju-connect.exe, no subprocess stdout scraping).
type App struct {
	ctx          context.Context
	mu           sync.Mutex
	appConfig    AppConfig
	status       VPNStatus
	connections  map[string]*vpnConnection
	starting     bool
	restarting   bool
	shuttingDown bool
	startTime    time.Time
	tickerCancel context.CancelFunc
	configPath   string
	resources    []engine.ResourceEntry
}

// NewApp creates a new App application struct
func NewApp() *App {
	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)

	configPath := filepath.Join(baseDir, "vpn_profiles.json")
	if _, err := os.Stat(configPath); err != nil {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			appDir := filepath.Join(appData, "EasyConnectVPN")
			_ = os.MkdirAll(appDir, 0755)
			configPath = filepath.Join(appDir, "vpn_profiles.json")
		}
	}

	app := &App{
		configPath: configPath,
		status: VPNStatus{
			Running:   false,
			State:     "disconnected",
			SocksPort: "1080",
			HTTPPort:  "1081",
		},
	}

	app.initDefaultConfig()
	app.loadConfigFile()
	app.normalizeProfiles()
	app.selectConnectionLocked()
	return app
}

func (a *App) initDefaultConfig() {
	defaultProfile := Profile{
		ID:               "profile-default",
		Name:             "示例配置（请编辑）",
		Server:           "",
		Port:             443,
		Username:         "",
		Password:         "",
		Enable2FA:        false,
		TotpSecret:       "",
		BindInterface:    "",
		DisableZjuConfig: true,
	}

	a.appConfig = AppConfig{
		ActiveProfileID: defaultProfile.ID,
		Profiles:        []Profile{defaultProfile},
	}
}

func (a *App) loadConfigFile() {
	data, err := os.ReadFile(a.configPath)
	if err == nil {
		var loaded AppConfig
		if err := json.Unmarshal(data, &loaded); err == nil && len(loaded.Profiles) > 0 {
			a.appConfig = loaded
		}
	}
}

func (a *App) saveConfigFile() error {
	data, err := json.MarshalIndent(a.appConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.configPath, data, 0644)
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.mu.Lock()
	a.starting = true
	a.mu.Unlock()
	a.handleEngineStatus(engine.Status{Phase: engine.PhaseConnecting, Stage: "恢复网络配置", Message: "正在检查并清理上次遗留的 TUN 配置…"})
	if restarting, err := a.cleanupTUNOnStartup(); restarting {
		runtime.Quit(ctx)
		return
	} else if err != nil {
		a.handleEngineStatus(engine.Status{Phase: engine.PhaseError, Message: err.Error()})
	} else {
		a.handleEngineStatus(engine.Status{Phase: engine.PhaseDisconnected, Message: "网络配置检查完成"})
	}

	// Start Windows system tray
	StartTray(a, ctx)

	// Start background status ticker
	tickerCtx, cancel := context.WithCancel(context.Background())
	a.tickerCancel = cancel

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-tickerCtx.Done():
				return
			case <-ticker.C:
				a.mu.Lock()
				for id, c := range a.connections {
					if c.status.Running && !c.started.IsZero() {
						c.status.Duration = int64(time.Since(c.started).Seconds())
						if id == a.appConfig.ActiveProfileID {
							a.status = c.status
						}
						runtime.EventsEmit(a.ctx, "status-tick", c.status)
					}
				}
				a.mu.Unlock()
			}
		}
	}()
	a.mu.Lock()
	a.starting = false
	a.mu.Unlock()
}

// shutdown cleans up resources
func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	a.shuttingDown = true
	a.mu.Unlock()
	RemoveTrayIcon()
	if a.tickerCancel != nil {
		a.tickerCancel()
	}
	_ = a.StopAllVPN()
}

// GetAppConfig returns all profiles and active ID
func (a *App) GetAppConfig() AppConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.appConfig
}

// GetActiveProfile returns the currently selected profile
func (a *App) GetActiveProfile() Profile {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.getActiveProfileLocked()
}

// SetActiveProfile switches active profile
func (a *App) SetActiveProfile(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, p := range a.appConfig.Profiles {
		if p.ID == id {
			a.appConfig.ActiveProfileID = id
			a.selectConnectionLocked()
			_ = a.saveConfigFile()
			return true
		}
	}
	return false
}

// SaveProfile creates or updates a profile
func (a *App) SaveProfile(p Profile) (Profile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.connections[p.ID]; c != nil && (c.status.Running || c.stopping) {
		return Profile{}, fmt.Errorf("请先断开该配置再编辑")
	}
	if p.Protocol == "" {
		p.Protocol = "easyconnect"
	}
	if p.Protocol != "easyconnect" && p.Protocol != "univpn" {
		return Profile{}, fmt.Errorf("不支持的 VPN 类型")
	}
	if p.Protocol == "univpn" && p.Enable2FA {
		return Profile{}, fmt.Errorf("UniVPN 当前仅支持账号密码认证")
	}
	if p.Port < 1 || p.Port > 65535 {
		return Profile{}, fmt.Errorf("服务器端口必须为 1–65535")
	}
	if p.ID == "" {
		p.ID = fmt.Sprintf("profile-%d", time.Now().UnixNano())
	}
	a.assignProxyPorts(&p)
	if p.SocksPort < 1 || p.SocksPort > 65535 || p.HTTPPort < 1 || p.HTTPPort > 65535 || p.SocksPort == p.HTTPPort {
		return Profile{}, fmt.Errorf("请填写两个不同的有效代理端口")
	}
	previous := AppConfig{ActiveProfileID: a.appConfig.ActiveProfileID, Profiles: append([]Profile(nil), a.appConfig.Profiles...)}
	found := false
	for i, existing := range a.appConfig.Profiles {
		if existing.ID == p.ID {
			a.appConfig.Profiles[i] = p
			found = true
			break
		}
	}
	if !found {
		a.appConfig.Profiles = append(a.appConfig.Profiles, p)
	}
	a.appConfig.ActiveProfileID = p.ID
	if err := a.saveConfigFile(); err != nil {
		a.appConfig = previous
		return Profile{}, err
	}
	delete(a.connections, p.ID)
	a.selectConnectionLocked()
	return p, nil
}

// DeleteProfile removes a profile by ID
func (a *App) DeleteProfile(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if c := a.connections[id]; c != nil && (c.status.Running || c.stopping) {
		return false
	}
	if len(a.appConfig.Profiles) <= 1 {
		return false // Preserve at least one profile
	}

	var newProfiles []Profile
	for _, p := range a.appConfig.Profiles {
		if p.ID != id {
			newProfiles = append(newProfiles, p)
		}
	}

	a.appConfig.Profiles = newProfiles
	if a.appConfig.ActiveProfileID == id && len(newProfiles) > 0 {
		a.appConfig.ActiveProfileID = newProfiles[0].ID
	}

	delete(a.connections, id)
	a.selectConnectionLocked()
	_ = a.saveConfigFile()
	return true
}

// GetStatus returns the current VPN state
func (a *App) GetStatus() VPNStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// SetProxyEnabled saves the local proxy setting for the selected profile.
func (a *App) SetProxyEnabled(enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.starting || a.status.Running || a.restarting {
		return fmt.Errorf("请先断开连接，再切换本地代理")
	}
	for i := range a.appConfig.Profiles {
		if a.appConfig.Profiles[i].ID != a.appConfig.ActiveProfileID {
			continue
		}
		previous := a.appConfig.Profiles[i].DisableProxy
		a.appConfig.Profiles[i].DisableProxy = !enabled
		if err := a.saveConfigFile(); err != nil {
			a.appConfig.Profiles[i].DisableProxy = previous
			return fmt.Errorf("保存代理配置失败：%w", err)
		}
		return nil
	}
	return fmt.Errorf("请先选择服务器配置")
}

// GetInterfaces returns available network adapter names
func (a *App) GetInterfaces() []string {
	var result []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return []string{"以太网", "WLAN"}
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 {
			result = append(result, iface.Name)
		}
	}
	if len(result) == 0 {
		return []string{"以太网", "WLAN"}
	}
	return result
}

// buildEngineConfig maps a GUI profile onto the upstream engine config.
func (a *App) buildEngineConfig(profile Profile) configs.Config {
	conf := configs.Default()
	if profile.Protocol != "" {
		conf.Protocol = profile.Protocol
	}

	conf.ServerAddress = profile.Server
	if profile.Port > 0 {
		conf.ServerPort = profile.Port
	}
	conf.Username = profile.Username
	conf.Password = profile.Password

	if profile.Enable2FA {
		conf.TOTPSecret = strings.TrimSpace(profile.TotpSecret)
	}
	conf.BindInterface = profile.BindInterface
	conf.AutoDetectInterface = profile.BindInterface == ""
	conf.TUNMode = profile.TUNMode
	conf.AddRoute = profile.TUNMode
	socks, http := profile.SocksPort, profile.HTTPPort
	if socks == 0 {
		socks = 1080
	}
	if http == 0 {
		http = 1081
	}
	conf.SocksBind = net.JoinHostPort("127.0.0.1", fmt.Sprint(socks))
	conf.HTTPBind = net.JoinHostPort("127.0.0.1", fmt.Sprint(http))
	if profile.DisableProxy {
		conf.SocksBind = ""
		conf.HTTPBind = ""
	}
	conf.DisableZJUConfig = profile.DisableZjuConfig || profile.TUNMode

	if profile.DisableKeepAlive {
		conf.DisableKeepAlive = true
	} else if url := strings.TrimSpace(profile.KeepAliveURL); url != "" {
		conf.KeepAliveURL = url
	}

	if conf.Protocol == "univpn" {
		conf.DisableRemoteDNS = true
		conf.DisableZJUConfig = true
		conf.DisableKeepAlive = true
	}
	return conf
}

// GetVPNResources returns the server-authorized resource list of the
// current connection (IP segments / domains with ports), mirroring the
// resource page of the official EasyConnect client.
func (a *App) GetVPNResources() []engine.ResourceEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.resources
}

// handleEngineStatus translates engine lifecycle events into the VPNStatus
// the frontend and tray already understand. Called from engine goroutines.
func (a *App) handleEngineStatus(s engine.Status) {
	a.mu.Lock()
	defer a.mu.Unlock()

	applyEngineStatus(&a.status, &a.startTime, &a.resources, s)

	a.syncTrayTooltipLocked()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "vpn-status", a.status)
	}
}

func (a *App) syncTrayTooltipLocked() {
	if a.anyRunningLocked() {
		count := 0
		for _, c := range a.connections {
			if c.status.Running {
				count++
			}
		}
		UpdateTrayTooltip(fmt.Sprintf("VPN 客户端\n%d 条连接运行中", count))
		return
	}
	var tip string
	if a.status.Running {
		if a.status.State == "connected" {
			if a.status.ClientIP != "" {
				tip = fmt.Sprintf("EasyConnect VPN\n已连接: %s\nIP: %s", a.status.ActiveProfile, a.status.ClientIP)
			} else {
				tip = fmt.Sprintf("EasyConnect VPN\n已连接: %s", a.status.ActiveProfile)
			}
		} else {
			tip = fmt.Sprintf("EasyConnect VPN\n%s: %s", a.status.Stage, a.status.ActiveProfile)
		}
	} else {
		tip = "EasyConnect VPN (未连接)"
	}
	UpdateTrayTooltip(tip)
}

func (a *App) getActiveProfileLocked() Profile {
	for _, p := range a.appConfig.Profiles {
		if p.ID == a.appConfig.ActiveProfileID {
			return p
		}
	}
	if len(a.appConfig.Profiles) > 0 {
		return a.appConfig.Profiles[0]
	}
	return Profile{}
}

func applyEngineStatus(status *VPNStatus, startTime *time.Time, resources *[]engine.ResourceEntry, s engine.Status) {
	status.Stage = s.Stage
	status.Message = s.Message
	switch s.Phase {
	case engine.PhaseConnecting:
		status.Running = true
		status.State = "connecting"
		status.LastError = ""

	case engine.PhaseReconnecting:
		status.Running = true
		status.State = "connecting"
		status.Stage = "重连中"
		status.LastError = ""

	case engine.PhaseConnected:
		status.Running = true
		status.State = "connected"
		status.LastError = ""
		if s.ClientIP != "" {
			status.ClientIP = s.ClientIP
		}
		if s.DNS != "" {
			status.DNS = s.DNS
		}
		if (*startTime).IsZero() {
			(*startTime) = time.Now()
		}

	case engine.PhaseDisconnected:
		status.Running = false
		status.State = "disconnected"
		(*startTime) = time.Time{}
		(*resources) = nil

	case engine.PhaseError:
		status.Running = false
		status.State = "error"
		status.LastError = s.Message
		(*startTime) = time.Time{}
		(*resources) = nil
	}

}
