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
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Server           string   `json:"server"`
	Port             int      `json:"port"`
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	Enable2FA        bool     `json:"enable2FA"`
	TotpSecret       string   `json:"totpSecret"`
	BindInterface    string   `json:"bindInterface"`
	TUNMode          bool     `json:"tunMode"`
	TUNDNSDomains    []string `json:"tunDnsDomains"`
	DisableZjuConfig bool     `json:"disableZjuConfig"`
	DisableKeepAlive bool     `json:"disableKeepAlive"`
	KeepAliveURL     string   `json:"keepAliveUrl"`
}

// AppConfig stores all profiles and active selection
type AppConfig struct {
	ActiveProfileID string    `json:"activeProfileId"`
	Profiles        []Profile `json:"profiles"`
}

// VPNStatus contains current connection status
type VPNStatus struct {
	Running       bool   `json:"running"`
	State         string `json:"state"` // "disconnected", "connecting", "connected", "error"
	ActiveProfile string `json:"activeProfile"`
	ClientIP      string `json:"clientIp"`
	DNS           string `json:"dns"`
	SocksPort     string `json:"socksPort"`
	HTTPPort      string `json:"httpPort"`
	TUNMode       bool   `json:"tunMode"`
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
	engine       *engine.Engine
	starting     bool
	restarting   bool
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
				if a.status.Running && !a.startTime.IsZero() {
					a.status.Duration = int64(time.Since(a.startTime).Seconds())
					runtime.EventsEmit(a.ctx, "status-tick", a.status)
				}
				a.mu.Unlock()
			}
		}
	}()
}

// shutdown cleans up resources
func (a *App) shutdown(ctx context.Context) {
	RemoveTrayIcon()
	if a.tickerCancel != nil {
		a.tickerCancel()
	}
	_ = a.StopVPN()
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
			_ = a.saveConfigFile()
			return true
		}
	}
	return false
}

// SaveProfile creates or updates a profile
func (a *App) SaveProfile(p Profile) Profile {
	a.mu.Lock()
	defer a.mu.Unlock()

	if p.ID == "" {
		p.ID = fmt.Sprintf("profile-%d", time.Now().UnixMilli())
		a.appConfig.Profiles = append(a.appConfig.Profiles, p)
		a.appConfig.ActiveProfileID = p.ID
	} else {
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
	}

	_ = a.saveConfigFile()
	return p
}

// DeleteProfile removes a profile by ID
func (a *App) DeleteProfile(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

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

	_ = a.saveConfigFile()
	return true
}

// GetStatus returns the current VPN state
func (a *App) GetStatus() VPNStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
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

// StartVPN starts the in-process VPN engine using the active profile.
// The login sequence runs synchronously (a few seconds); progress is
// reported through vpn-status / vpn-log events.
func (a *App) StartVPN() string {
	a.mu.Lock()
	if a.restarting {
		a.mu.Unlock()
		return "正在以管理员身份重新启动"
	}
	if a.engine != nil && a.engine.Running() {
		a.mu.Unlock()
		return "Already running"
	}
	if a.starting {
		a.mu.Unlock()
		return "正在启动中，请稍候..."
	}

	profile := a.getActiveProfileLocked()
	if profile.Server == "" || profile.Username == "" {
		a.mu.Unlock()
		return "配置不完整：服务器地址或用户名为空"
	}

	a.starting = true
	a.startTime = time.Now()
	a.status = VPNStatus{
		Running:       true,
		State:         "connecting",
		ActiveProfile: profile.Name,
		TUNMode:       profile.TUNMode,
		SocksPort:     "1080",
		HTTPPort:      "1081",
	}
	a.syncTrayTooltipLocked()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "vpn-status", a.status)
	}

	logDir := ""
	if appData := os.Getenv("APPDATA"); appData != "" {
		logDir = filepath.Join(appData, "EasyConnectVPN", "logs")
		_ = os.MkdirAll(logDir, 0755)
	} else {
		logDir = "."
	}

	opts := engine.Options{
		Config:      a.buildEngineConfig(profile),
		OnStatus:    a.handleEngineStatus,
		OnResources: a.handleResources,
		LogFile:     filepath.Join(logDir, "engine.log"),
	}
	a.mu.Unlock()

	eng, err := engine.Start(opts)

	a.mu.Lock()
	a.starting = false
	if err != nil {
		a.engine = nil
		a.startTime = time.Time{}
		a.status.Running = false
		a.status.State = "error"
		a.status.LastError = err.Error()
		a.syncTrayTooltipLocked()
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "vpn-status", a.status)
		}
		a.mu.Unlock()
		return err.Error()
	}

	a.engine = eng
	a.resources = eng.Resources()
	if a.ctx != nil {
		// Push the resource list after it is actually stored: the
		// "connected" vpn-status event fires mid-login, before this
		// assignment, so a frontend fetch triggered by it races empty.
		runtime.EventsEmit(a.ctx, "vpn-resources", a.resources)
	}
	a.mu.Unlock()
	return "OK"
}

// buildEngineConfig maps a GUI profile onto the upstream engine config.
func (a *App) buildEngineConfig(profile Profile) configs.Config {
	conf := configs.Default()

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
	conf.TUNDNSDomains = append([]string(nil), profile.TUNDNSDomains...)
	conf.AddRoute = profile.TUNMode
	conf.SocksBind = "127.0.0.1:1080"
	conf.HTTPBind = "127.0.0.1:1081"
	conf.DisableZJUConfig = profile.DisableZjuConfig || profile.TUNMode

	if profile.DisableKeepAlive {
		conf.DisableKeepAlive = true
	} else if url := strings.TrimSpace(profile.KeepAliveURL); url != "" {
		conf.KeepAliveURL = url
	}

	return conf
}

func (a *App) handleResources(entries []engine.ResourceEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resources = append([]engine.ResourceEntry(nil), entries...)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "vpn-resources", a.resources)
	}
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

	switch s.Phase {
	case engine.PhaseConnecting:
		a.status.Running = true
		a.status.State = "connecting"

	case engine.PhaseReconnecting:
		a.status.Running = true
		a.status.State = "connecting"
		a.status.LastError = s.Message

	case engine.PhaseConnected:
		a.status.Running = true
		a.status.State = "connected"
		a.status.LastError = ""
		if s.ClientIP != "" {
			a.status.ClientIP = s.ClientIP
		}
		if s.DNS != "" {
			a.status.DNS = s.DNS
		}
		if a.startTime.IsZero() {
			a.startTime = time.Now()
		}

	case engine.PhaseDisconnected:
		a.status.Running = false
		a.status.State = "disconnected"
		a.startTime = time.Time{}
		a.resources = nil

	case engine.PhaseError:
		a.status.Running = false
		a.status.State = "error"
		a.status.LastError = s.Message
		a.startTime = time.Time{}
		a.resources = nil
	}

	a.syncTrayTooltipLocked()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "vpn-status", a.status)
	}
}

func (a *App) syncTrayTooltipLocked() {
	var tip string
	if a.status.Running {
		if a.status.State == "connected" {
			if a.status.ClientIP != "" {
				tip = fmt.Sprintf("EasyConnect VPN\n已连接: %s\nIP: %s", a.status.ActiveProfile, a.status.ClientIP)
			} else {
				tip = fmt.Sprintf("EasyConnect VPN\n已连接: %s", a.status.ActiveProfile)
			}
		} else {
			tip = fmt.Sprintf("EasyConnect VPN\n握手中: %s...", a.status.ActiveProfile)
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

// StopVPN stops the in-process VPN engine
func (a *App) StopVPN() string {
	a.mu.Lock()
	eng := a.engine
	a.mu.Unlock()

	if eng == nil || !eng.Running() {
		a.mu.Lock()
		a.status.Running = false
		a.status.State = "disconnected"
		a.startTime = time.Time{}
		a.syncTrayTooltipLocked()
		a.mu.Unlock()
		return "Not running"
	}

	// Stop runs the teardown chain and emits PhaseDisconnected, which
	// updates status + tray through handleEngineStatus.
	eng.Stop()
	return "OK"
}
