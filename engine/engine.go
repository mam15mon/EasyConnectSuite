// Package engine adapts the zju-connect engine (originally a standalone
// CLI in package main) into a library that the EasyConnect Suite GUI can
// start, stop and observe in-process. Only the EasyConnect protocol over
// the gVisor stack is wired up — the profile editor does not expose the
// aTrust protocol, TUN mode or the TCP-tunnel stack.
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
	"sync"
	"sync/atomic"
	"time"

	"github.com/mythologyli/zju-connect/client"
	easyconnectclient "github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/lifecycle"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/mythologyli/zju-connect/service"
	"github.com/mythologyli/zju-connect/stack/gvisor"
	"github.com/mythologyli/zju-connect/underlay"
	"inet.af/netaddr"
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

type Options struct {
	Config configs.Config

	// OnLog receives every log line produced by the engine.
	OnLog func(line string)
	// OnStatus receives lifecycle transitions.
	OnStatus func(Status)
	// LogFile, when set, receives a persistent copy of every engine log
	// line so post-mortem analysis survives a UI-level crash.
	LogFile string
}

type Engine struct {
	conf configs.Config
	opts Options

	running atomic.Bool

	dnsName string

	stopOnce sync.Once
	logClose io.Closer
	logFile  *os.File
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

	e.startLogCapture()

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
		e.stopLogCapture()
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
		e.stopLogCapture()
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

	ipResources, err := vpnClient.IPResources()
	if err != nil && !conf.DisableServerConfig {
		log.Println("No IP resources")
	}

	ipSet, err := vpnClient.IPSet()
	if err != nil && !conf.DisableServerConfig {
		log.Println("No IP set")
	}

	domainResources, err := vpnClient.DomainResources()
	if err != nil && !conf.DisableServerConfig {
		log.Println("No domain resources")
	}

	dnsResource, err := vpnClient.DNSResource()
	if err != nil && !conf.DisableServerConfig {
		log.Println("No DNS resource")
	}

	if !conf.DisableZJUConfig {
		if domainResources == nil {
			domainResources = make(client.DomainResources)
		}
		domainResources["zju.edu.cn"] = []client.DomainResource{{
			PortMin: 1, PortMax: 65535, Protocol: "all",
		}}

		if ipResources == nil {
			ipResources = []client.IPResource{}
		}
		ipResources = append([]client.IPResource{{
			IPMin: net.ParseIP("10.0.0.0"), IPMax: net.ParseIP("10.255.255.255"),
			PortMin: 1, PortMax: 65535, Protocol: "all",
		}}, ipResources...)

		ipSetBuilder := netaddr.IPSetBuilder{}
		if ipSet != nil {
			ipSetBuilder.AddSet(ipSet)
		}
		ipSetBuilder.AddPrefix(netaddr.MustParseIPPrefix("10.0.0.0/8"))
		ipSet, _ = ipSetBuilder.IPSet()
	}

	for _, customProxyDomain := range conf.CustomProxyDomain {
		if domainResources != nil {
			domainResources[customProxyDomain] = append(domainResources[customProxyDomain], client.DomainResource{
				PortMin: 1, PortMax: 65535, Protocol: "all",
			})
		} else {
			domainResources = client.DomainResources{
				customProxyDomain: {{PortMin: 1, PortMax: 65535, Protocol: "all"}},
			}
		}
	}

	e.emitStatus(PhaseConnecting, "创建虚拟网络栈...")

	// The GUI always uses the gVisor L3 stack (upstream default).
	vpnStack, err := gvisor.NewStack(vpnClient)
	if err != nil {
		vpnClient.Close()
		_ = underlayDialer.Close()
		e.stopLogCapture()
		return nil, fmt.Errorf("gVisor 网络栈创建失败: %w", err)
	}

	vpnStack.SetupRelogin(func() error {
		e.emitStatus(PhaseReconnecting, "连接中断，正在自动重新登录...")
		setupErr := vpnClient.Setup()
		if setupErr != nil {
			return setupErr
		}
		clientIP := ""
		if ip, ipErr := vpnClient.IP(); ipErr == nil {
			clientIP = ip.String()
		}
		e.emitStatusFull(Status{Phase: PhaseConnected, Message: "重新登录成功，隧道已重建", ClientIP: clientIP, DNS: e.dnsName})
		return nil
	})
	vpnStack.SetupFatalHandler(e.handleTunnelFatal)

	e.registerTerminal("CloseVPNStack", func(ctx context.Context) error {
		return vpnStack.Close()
	})

	useRemoteDNS := !conf.DisableRemoteDNS
	remoteDNSServer := conf.RemoteDNSServer
	policyDNSServers, _ := vpnClient.DNSServers()
	if useRemoteDNS && remoteDNSServer == "auto" {
		remoteDNSServer, err = vpnClient.DNSServer()
		if err != nil {
			useRemoteDNS = false
			remoteDNSServer = "10.10.0.21"
			log.Println("No DNS server provided by server. Disable remote DNS")
		} else {
			log.Printf("Use DNS server %s provided by server", remoteDNSServer)
		}
	}
	e.dnsName = remoteDNSServer

	secondaryDNSServer := conf.SecondaryDNSServer
	if secondaryDNSServer == "auto" {
		secondaryDNSServer = "114.114.114.114"
		if len(policyDNSServers) > 1 {
			secondaryDNSServer = policyDNSServers[1]
			log.Printf("Use secondary DNS server %s provided by server", secondaryDNSServer)
		}
	}

	vpnResolver := resolve.NewResolver(
		vpnStack,
		remoteDNSServer,
		secondaryDNSServer,
		conf.DNSTTL,
		domainResources,
		dnsResource,
		useRemoteDNS,
	)
	e.registerTerminal("CloseResolver", func(ctx context.Context) error {
		vpnResolver.Close()
		return nil
	})

	for _, customDns := range conf.CustomDNSList {
		ipAddr := net.ParseIP(customDns.IP)
		if ipAddr == nil {
			log.Printf("Custom DNS for host name %s is invalid, SKIP", customDns.HostName)
			continue
		}
		vpnResolver.SetPermanentDNS(customDns.HostName, ipAddr)
		log.Printf("Add custom DNS: %s -> %s", customDns.HostName, customDns.IP)
	}
	localResolver := service.NewDnsServer(vpnResolver, []string{remoteDNSServer, conf.SecondaryDNSServer})
	vpnStack.SetupResolve(localResolver)
	vpnStack.SetupIPPool(vpnResolver.IPPool)

	e.goWithRecovery("gvisor stack Run", func() {
		vpnStack.Run()
	})

	vpnDialer := dial.NewDialer(vpnStack, vpnResolver, ipResources, conf.ProxyAll, conf.DialDirectProxy)

	if conf.SocksBind != "" {
		e.goWithRecovery("SOCKS5 server", func() {
			service.ServeSocks5(conf.SocksBind, vpnDialer, vpnResolver, conf.SocksUser, conf.SocksPasswd)
		})
	}
	if conf.HTTPBind != "" {
		e.goWithRecovery("HTTP proxy server", func() {
			service.ServeHTTP(conf.HTTPBind, vpnDialer)
		})
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

	e.running.Store(true)
	e.emitStatusFull(Status{Phase: PhaseConnected, Message: "VPN 已连接", ClientIP: clientIP, DNS: e.dnsName})
	return e, nil
}

// Running reports whether the engine is up.
func (e *Engine) Running() bool {
	return e.running.Load()
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
		if f := openEngineLogFile(e.opts.LogFile); f != nil {
			e.logFile = f
			writers = append(writers, f)
		}
	}

	if len(writers) > 0 {
		if len(writers) == 1 {
			stdlog.SetOutput(writers[0])
		} else {
			stdlog.SetOutput(io.MultiWriter(writers...))
		}
	}
}

// openEngineLogFile opens (or rotates) the persistent engine log.
func openEngineLogFile(path string) *os.File {
	if info, err := os.Stat(path); err == nil && info.Size() > 8*1024*1024 {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	_, _ = f.WriteString(fmt.Sprintf("\n=== engine session started %s ===\n", time.Now().Format(time.RFC3339)))
	return f
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
	if e.logFile != nil {
		_, _ = e.logFile.WriteString(fmt.Sprintf("=== engine session ended %s ===\n", time.Now().Format(time.RFC3339)))
		_ = e.logFile.Close()
		e.logFile = nil
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
