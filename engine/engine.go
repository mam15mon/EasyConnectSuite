// Package engine adapts the zju-connect engine (originally a standalone
// CLI in package main) into a library that the EasyConnect Suite GUI can
// start, stop and observe in-process. Optional Windows TUN ingress shares
// the existing gVisor VPN engine and its reconnect handling.
package engine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	stdlog "log"
	"net"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mythologyli/zju-connect/client"
	easyconnectclient "github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/lifecycle"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/service"
	"github.com/mythologyli/zju-connect/stack/gvisor"
	"github.com/mythologyli/zju-connect/underlay"
)

// Status phases reported through Options.OnStatus.
const (
	PhaseConnecting   = "connecting"
	PhaseConnected    = "connected"
	PhaseReconnecting = "reconnecting"
	PhaseDisconnected = "disconnected"
	PhaseError        = "error"
)

type Status struct {
	Phase    string
	Message  string
	ClientIP string
	DNS      string
}

// ResourceEntry is one row of the server-authorized resource list shown in
// the GUI (mirrors the resource page of the official client).
// The json tags matter: Wails serializes these straight to the frontend,
// which reads the lowercase field names.
type ResourceEntry struct {
	Kind     string `json:"kind"`     // "ip" or "domain"
	Target   string `json:"target"`   // single IP, IP range or domain name
	Ports    string `json:"ports"`    // human-readable port range
	Protocol string `json:"protocol"` // human-readable protocol
}

type Options struct {
	Config configs.Config

	// OnLog receives every log line produced by the engine.
	OnLog func(line string)
	// OnStatus receives lifecycle transitions.
	OnStatus    func(Status)
	OnResources func([]ResourceEntry)
	// LogFile, when set, receives a persistent copy of every engine log
	// line so post-mortem analysis survives a UI-level crash.
	LogFile string
}

type Engine struct {
	conf configs.Config
	opts Options

	running atomic.Bool

	dnsName string
	router  policyRouter
	tun     atomic.Pointer[tunIngress]
	fatal   atomic.Bool

	stopOnce  sync.Once
	logClose  io.Closer
	logWriter *rotatingWriter
}

// Start performs the whole login + service bring-up synchronously (the
// same sequence upstream main.go runs) and returns a running engine.
// A returned error means login/setup failed and nothing is left running.
func Start(opts Options) (*Engine, error) {
	conf := opts.Config

	if conf.ServerAddress == "" || conf.Username == "" {
		return nil, fmt.Errorf("服务器地址或用户名为空")
	}

	e := &Engine{conf: conf, opts: opts}

	lifecycle.ResetTerminalFuncs()
	e.startLogCapture()
	started := false
	defer func() {
		if !started {
			e.execTerminal()
			e.stopLogCapture()
		}
	}()

	// A previous engine run may have marked the global terminal registry
	// as shut down; clear it so this run can register its hooks.
	lifecycle.ResetTerminalFuncs()

	e.emitStatus(PhaseConnecting, "初始化网络...")

	underlayDialer, err := underlay.New(underlay.Options{
		InterfaceName:  conf.BindInterface,
		AutoDetect:     conf.AutoDetectInterface,
		DebugPCAPFile:  conf.DebugPCAPFile,
		LocalDNSServer: conf.LocalDNSServer,
	})
	if err != nil {
		return nil, fmt.Errorf("创建底层拨号器失败: %w", err)
	}

	// TLS key logging is a debug feature of the CLI (internal/keylog is
	// not importable from outside the module); the GUI passes nil.
	vpnClient := easyconnectclient.NewClient(easyconnectclient.Options{
		Server: net.JoinHostPort(conf.ServerAddress, strconv.Itoa(conf.ServerPort)),
		Auth: easyconnectclient.AuthOptions{
			Username:   conf.Username,
			Password:   conf.Password,
			TOTPSecret: conf.TOTPSecret,
			// Certificate / GraphCodeFile left empty: the GUI profile
			// model only supports username+password(+TOTP).
		},
		SessionID:     conf.TwfID,
		TestMultiLine: !conf.DisableMultiLine,
		Resources: easyconnectclient.ResourceOptions{
			Fetch:          !conf.DisableServerConfig,
			IncludeDomains: !conf.SkipDomainResource,
		},
		UnderlayDialer:  underlayDialer,
		TLSKeyLogWriter: nil,
	})

	log.Printf("VPN protocol: %s", conf.Protocol)
	e.emitStatus(PhaseConnecting, "登录 VPN 服务器...")

	if err := vpnClient.Setup(); err != nil {
		vpnClient.Close()
		_ = underlayDialer.Close()
		return nil, fmt.Errorf("VPN 登录失败: %w", err)
	}

	log.Printf("VPN client started")
	e.registerTerminal("CloseVPNClient", func(ctx context.Context) error {
		vpnClient.Close()
		return nil
	})
	e.registerTerminal("CloseUnderlayDialer", func(ctx context.Context) error {
		return underlayDialer.Close()
	})

	e.emitStatus(PhaseConnecting, "创建虚拟网络栈...")

	// The GUI always uses the gVisor L3 stack (upstream default).
	vpnStack, err := gvisor.NewStack(vpnClient)
	if err != nil {
		vpnClient.Close()
		_ = underlayDialer.Close()
		return nil, fmt.Errorf("gVisor 网络栈创建失败: %w", err)
	}

	e.registerTerminal("CloseVPNStack", func(ctx context.Context) error { return vpnStack.Close() })
	policy, err := loadPolicy(conf, vpnClient, vpnStack)
	if err != nil {
		return nil, err
	}
	e.router.current.Store(policy)
	e.dnsName = policy.dnsName
	e.registerTerminal("CloseResolver", func(ctx context.Context) error {
		if p := e.router.current.Load(); p != nil {
			p.resolver.Close()
		}
		return nil
	})
	vpnStack.SetupIPPool(policy.resolver.IPPool)
	vpnStack.SetupRelogin(func() error {
		e.emitStatus(PhaseReconnecting, "连接中断，正在自动重新登录...")
		if err := vpnClient.Setup(); err != nil {
			return err
		}
		next, err := loadPolicy(conf, vpnClient, vpnStack)
		if err != nil {
			return err
		}
		if ingress := e.tun.Load(); ingress != nil {
			if err := ingress.Update(next); err != nil {
				next.resolver.Close()
				return err
			}
		}
		previous := e.router.current.Swap(next)
		vpnStack.SetupIPPool(next.resolver.IPPool)
		if previous != nil {
			previous.resolver.Close()
		}
		e.dnsName = next.dnsName
		if e.opts.OnResources != nil {
			e.opts.OnResources(next.entries)
		}

		return nil
	})
	vpnStack.SetupRecovered(func() {
		policy := e.router.current.Load()
		if ingress := e.tun.Load(); ingress != nil {
			e.goWithRecovery("TUN DNS zone refresh", func() {
				if err := ingress.RefreshDNS(policy); err != nil {
					log.Printf("TUN DNS zone refresh failed: %v", err)
				}
			})
		}
		clientIP := ""
		if ip, err := vpnClient.IP(); err == nil {
			clientIP = ip.String()
		}
		e.emitStatusFull(Status{Phase: PhaseConnected, Message: "隧道已重建，资源已更新", ClientIP: clientIP, DNS: policy.dnsName})
	})
	vpnStack.SetupFatalHandler(e.handleTunnelFatal)
	e.goWithRecovery("gvisor stack Run", func() { vpnStack.Run() })
	if conf.TUNMode {
		e.emitStatus(PhaseConnecting, "创建资源 TUN 并配置路由...")
		ingress, err := startTUN(&e.router, policy)
		if err != nil {
			return nil, fmt.Errorf("TUN 启动失败: %w", err)
		}
		e.tun.Store(ingress)
		e.registerTerminal("CloseTUNIngress", func(ctx context.Context) error { return ingress.Close() })
	}
	vpnDialer := &e.router
	vpnResolver := &e.router
	useRemoteDNS := policy.useRemoteDNS
	if conf.SocksBind != "" {
		e.goWithRecovery("SOCKS5 server", func() { service.ServeSocks5(conf.SocksBind, vpnDialer, vpnResolver, conf.SocksUser, conf.SocksPasswd) })
	}
	if conf.HTTPBind != "" {
		e.goWithRecovery("HTTP proxy server", func() { service.ServeHTTP(conf.HTTPBind, vpnDialer) })
	}

	for _, portForwarding := range conf.PortForwardingList {
		switch portForwarding.NetworkType {
		case "tcp":
			pf := portForwarding
			e.goWithRecovery("TCP forwarding "+pf.BindAddress, func() {
				service.ServeTCPForwarding(vpnDialer.Dial, pf.BindAddress, pf.RemoteAddress)
			})
		case "udp":
			pf := portForwarding
			e.goWithRecovery("UDP forwarding "+pf.BindAddress, func() {
				service.ServeUDPForwarding(vpnStack, pf.BindAddress, pf.RemoteAddress)
			})
		default:
			log.Printf("Port forwarding: unknown network type %s. Aborting", portForwarding.NetworkType)
		}
	}

	if !conf.DisableKeepAlive {
		if conf.KeepAliveURL == "" && !useRemoteDNS {
			log.Println("Keep alive is disabled because remote DNS is disabled, and no KeepAliveURL is provided")
		} else {
			keepAliveCtx, keepAliveCancel := context.WithCancel(context.Background())
			e.registerTerminal("CloseKeepAlive", func(ctx context.Context) error {
				keepAliveCancel()
				return nil
			})
			e.goWithRecovery("keepalive", func() {
				service.KeepAlive(keepAliveCtx, vpnResolver, vpnDialer, conf.KeepAliveURL)
			})
		}
	}

	clientIP := ""
	if ip, ipErr := vpnClient.IP(); ipErr == nil {
		clientIP = ip.String()
	}

	if e.fatal.Load() {
		return nil, fmt.Errorf("VPN 隧道在启动期间断开")
	}
	e.running.Store(true)
	e.emitStatusFull(Status{Phase: PhaseConnected, Message: "VPN 已连接", ClientIP: clientIP, DNS: e.dnsName})
	started = true
	return e, nil
}

// Running reports whether the engine is up.
func (e *Engine) Running() bool {
	return e.running.Load()
}

// Resources returns the server-authorized resource list for display.
func (e *Engine) Resources() []ResourceEntry {
	if p := e.router.current.Load(); p != nil {
		return append([]ResourceEntry(nil), p.entries...)
	}
	return nil
}

// buildResourceEntries flattens the parsed server resources into
// display rows.
func buildResourceEntries(ipResources []client.IPResource, domainResources client.DomainResources, dnsResource map[string][]net.IP) []ResourceEntry {
	entries := make([]ResourceEntry, 0, len(ipResources)+len(domainResources)+len(dnsResource))

	for _, r := range ipResources {
		entries = append(entries, ResourceEntry{
			Kind:     "ip",
			Target:   formatIPRange(r.IPMin, r.IPMax),
			Ports:    formatPortRange(r.PortMin, r.PortMax),
			Protocol: formatProtocol(r.Protocol),
		})
	}
	for domain, rules := range domainResources {
		ports, proto := "全部端口", "全部协议"
		if len(rules) > 0 {
			ports = formatPortRange(rules[0].PortMin, rules[0].PortMax)
			proto = formatProtocol(rules[0].Protocol)
		}
		entries = append(entries, ResourceEntry{
			Kind:     "domain",
			Target:   domain,
			Ports:    ports,
			Protocol: proto,
		})
	}
	for host, ips := range dnsResource {
		if len(ips) > 0 {
			entries = append(entries, ResourceEntry{
				Kind:     "domain",
				Target:   host + " → " + ips[0].String(),
				Ports:    "固定解析",
				Protocol: "-",
			})
		}
	}
	return entries
}

func formatIPRange(ipMin, ipMax net.IP) string {
	if ipMin == nil {
		return "-"
	}
	if ipMax == nil || ipMin.Equal(ipMax) {
		return ipMin.String()
	}
	return ipMin.String() + " ~ " + ipMax.String()
}

func formatPortRange(portMin, portMax int) string {
	if portMin <= 1 && portMax >= 65535 {
		return "全部端口"
	}
	if portMin == portMax {
		return strconv.Itoa(portMin)
	}
	return strconv.Itoa(portMin) + "-" + strconv.Itoa(portMax)
}

func formatProtocol(protocol string) string {
	switch protocol {
	case "", "all":
		return "全部协议"
	default:
		return strings.ToUpper(protocol)
	}
}

// Stop tears the engine down through the upstream terminal-hook chain
// (VPN client, underlay dialer, resolver, gvisor stack, SOCKS/HTTP/DNS
// listeners, keepalive). Safe to call multiple times.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		e.running.Store(false)
		log.Println("Shutdown ZJU-Connect ......")
		e.execTerminal()
		e.emitStatus(PhaseDisconnected, "VPN 已断开")
		e.stopLogCapture()
	})
}

// handleTunnelFatal is invoked by the gVisor stack when the L3 tunnel
// cannot be recovered even after the re-login campaign. It must never
// kill the process — just clean up and surface the error.
func (e *Engine) handleTunnelFatal(cause error) {
	e.fatal.Store(true)
	e.stopOnce.Do(func() {
		e.running.Store(false)
		log.Printf("Tunnel unrecoverable: %v; shutting down", cause)
		e.execTerminal()
		e.emitStatus(PhaseError, fmt.Sprintf("连接已断开且自动重连失败: %v", cause))
		e.stopLogCapture()
	})
}

func (e *Engine) execTerminal() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, err := range lifecycle.ExecTerminalFunc(ctx) {
		log.Printf("Shutdown ZJU-Connect failed: %s", err)
	}
}

func (e *Engine) registerTerminal(name string, f lifecycle.TerminalFunc) {
	lifecycle.RegisterTerminalFunc(name, f)
}

func (e *Engine) emitStatus(phase, message string) {
	e.emitStatusFull(Status{Phase: phase, Message: message})
}

func (e *Engine) emitStatusFull(s Status) {
	if e.opts.OnStatus != nil {
		e.opts.OnStatus(s)
	}
}

// startLogCapture redirects the engine log (upstream zju-connect logs
// through the standard library logger) into an OnLog callback so the GUI
// can show the same stream it used to scrape from the subprocess stdout,
// and — when Options.LogFile is set — tees every line into a persistent
// file so a crash leaves an activity trail behind.
func (e *Engine) startLogCapture() {
	var writers []io.Writer

	if e.opts.OnLog != nil {
		pr, pw := io.Pipe()
		e.logClose = pw
		writers = append(writers, pw)
		go func() {
			scanner := bufio.NewScanner(pr)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				e.opts.OnLog(scanner.Text())
			}
		}()
	}

	if e.opts.LogFile != "" {
		if w, err := newRotatingWriter(e.opts.LogFile, engineLogLimit); err == nil {
			e.logWriter = w
			writers = append(writers, w)
		}
	}

	if len(writers) > 0 {
		if len(writers) == 1 {
			stdlog.SetOutput(writers[0])
		} else {
			stdlog.SetOutput(io.MultiWriter(writers...))
		}
		stdlog.Printf("=== engine session started %s ===", time.Now().Format(time.RFC3339))
	}
}

// engineLogLimit caps engine.log; one .old generation is kept, so the log
// directory holds at most ~2x this size forever.
const engineLogLimit = 8 * 1024 * 1024

// rotatingWriter is a size-capped file writer: when the current file
// reaches limit bytes it is renamed to path+".old" (single generation,
// previous .old deleted) and a fresh file is opened — even mid-session,
// so months of uptime cannot grow the log without bound.
type rotatingWriter struct {
	path  string
	limit int64

	mu   sync.Mutex
	file *os.File
	size int64
}

func newRotatingWriter(path string, limit int64) (*rotatingWriter, error) {
	if limit <= 0 {
		limit = engineLogLimit
	}
	w := &rotatingWriter{path: path, limit: limit}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) open() error {
	if info, err := os.Stat(w.path); err == nil && info.Size() >= w.limit {
		_ = os.Rename(w.path, w.path+".old")
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}

	if w.size+int64(len(p)) > w.limit {
		_ = w.file.Close()
		_ = os.Remove(w.path + ".old")
		if err := os.Rename(w.path, w.path+".old"); err != nil {
			// Rotation failed (e.g. file locked by a viewer): keep
			// appending to the same file rather than losing lines.
			if err := w.open(); err != nil {
				return 0, err
			}
		} else if err := w.open(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (e *Engine) stopLogCapture() {
	// Closing the writer ends the scanner loop above. Service goroutines
	// that log during teardown will have their lines dropped silently by
	// stdlog (it ignores write errors) — acceptable once the GUI has
	// stopped listening.
	if e.logClose != nil {
		_ = e.logClose.Close()
		e.logClose = nil
	}
	if e.logWriter != nil {
		stdlog.Printf("=== engine session ended %s ===", time.Now().Format(time.RFC3339))
		_ = e.logWriter.Close()
		e.logWriter = nil
	}
}

// goWithRecovery runs fn in a goroutine that logs (instead of silently
// dying) if the code panics. Panics in goroutines kill the whole process
// otherwise — the exact overnight failure mode we are guarding against.
func (e *Engine) goWithRecovery(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("engine goroutine %q panicked: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}
