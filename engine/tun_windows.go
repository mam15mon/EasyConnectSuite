//go:build windows

package engine

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"EasyConnectSuite/internal/tunipc"
	"EasyConnectSuite/vpncore/log"
	"github.com/miekg/dns"
	stun "github.com/mythologyli/sing-tun"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"golang.org/x/sys/windows"
)

const tunName = "EasyConnect Suite"
const tunPrefix = "198.19.128.1/30"
const tunDNS = "198.19.128.2"
const nrptTag = "EasyConnectSuite resource TUN"

type tunIngress struct {
	name, dns, prefix string
	mu                sync.RWMutex
	device            stun.Tun
	stack             stun.Stack
	service           TUNBackend
	spec              TUNSpec
	routes            *routeManager
	backend           *windowsRoutes
	ctx               context.Context
	cancel            context.CancelFunc
	closeOnce         sync.Once
	closeErr          error
	closed            bool
	connections       sync.Map
}

// sing-tun already embeds Wintun. The ingress converts TCP/UDP flows into
// calls to our existing VPN dialer, so gVisor reconnect and IP updates remain
// shared with SOCKS5. It never changes the system's default route.
func StartTUNIngress(spec TUNSpec, service TUNBackend, progress func(string, string)) (_ *tunIngress, err error) {
	name, prefixText, dnsIP, tag, err := tunIdentity(spec.Slot)
	if err != nil {
		return nil, err
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return nil, fmt.Errorf("请以管理员身份运行 EasyConnectGUI 后启用 TUN")
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &tunIngress{name: name, dns: dnsIP, prefix: prefixText, service: service, ctx: ctx, cancel: cancel}
	defer func() {
		if err != nil {
			_ = t.Close()
		}
	}()
	progress("创建 TUN", "正在创建 TUN 虚拟网卡…")
	prefix := netip.MustParsePrefix(prefixText)
	t.device, err = stun.New(stun.Options{Name: name, Inet4Address: []netip.Prefix{prefix}, MTU: 1400, AutoRoute: false, StrictRoute: false, Logger: tunLogger{}})
	if err != nil {
		return nil, err
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	t.backend = &windowsRoutes{index: iface.Index, tag: tag, dns: dnsIP}
	t.routes = newRouteManager(t.backend)
	progress("配置网卡", "正在配置 TUN 网卡和接口优先级…")
	if err = t.backend.configure(); err != nil {
		return nil, err
	}
	t.stack, err = stun.NewStack("system", stun.StackOptions{Context: ctx, Tun: t.device, Name: name, MTU: 1400, Inet4Address: []netip.Prefix{prefix}, UDPTimeout: 60, Handler: t, Logger: tunLogger{}})
	if err != nil {
		return nil, err
	}
	if err = t.stack.Start(); err != nil {
		return nil, err
	}
	progress("识别 DNS", "正在通过 VPN DNS 识别内网域名区域…")
	if err = t.update(spec, progress); err != nil {
		return nil, err
	}
	log.Printf("TUN ready: %s; route metric 0, interface metric 0; DNS %s", name, dnsIP)
	return t, nil
}

func (t *tunIngress) UpdateSpec(spec TUNSpec) error { return t.update(spec, nil) }
func (t *tunIngress) update(spec TUNSpec, progress func(string, string)) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	prefixes := append(append([]string(nil), spec.Prefixes...), t.dnsAddress()+"/32")
	tunPolicies.Lock()
	defer tunPolicies.Unlock()
	if err := checkTUNConflicts(t, prefixes, spec.Names); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return net.ErrClosed
	}
	if progress != nil {
		progress("配置路由", "正在配置 VPN 资源路由…")
	}
	if err := t.routes.replace(prefixes); err != nil {
		return err
	}
	if err := t.backend.setDNS(append([]string(nil), spec.Names...)); err != nil {
		return err
	}
	t.spec = spec
	tunPolicies.claims[t] = tunClaim{prefixes: prefixes, names: spec.Names}
	return nil
}

func (t *tunIngress) Close() error {
	t.closeOnce.Do(func() {
		tunPolicies.Lock()
		t.mu.Lock()
		t.closed = true
		t.mu.Unlock()
		tunPolicies.Unlock()
		// Keep resource claims until teardown completes, but let pending DNS
		// handlers observe closed before waiting for the stack to stop.
		defer func() {
			tunPolicies.Lock()
			delete(tunPolicies.claims, t)
			tunPolicies.Unlock()
		}()
		t.cancel()
		t.connections.Range(func(key, _ any) bool { _ = key.(io.Closer).Close(); return true })
		if t.backend != nil {
			t.closeErr = errors.Join(t.closeErr, t.backend.setDNS(nil))
		}
		if t.routes != nil {
			t.closeErr = errors.Join(t.closeErr, t.routes.close())
		}
		if t.device != nil {
			t.closeErr = errors.Join(t.closeErr, t.device.Close())
		}
		if t.stack != nil {
			t.closeErr = errors.Join(t.closeErr, t.stack.Close())
		}
	})
	return t.closeErr
}

func (t *tunIngress) track(conn io.Closer) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.closed {
		_ = conn.Close()
		return false
	}
	t.connections.Store(conn, true)
	return true
}

func (t *tunIngress) dial(ctx context.Context, network string, target M.Socksaddr) (net.Conn, error) {
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return nil, net.ErrClosed
	}
	return t.service.Dial(ctx, network, target.String())
}

func (t *tunIngress) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	defer conn.Close()
	if !t.track(conn) {
		return net.ErrClosed
	}
	defer t.connections.Delete(conn)
	if metadata.Destination.Addr.String() == t.dnsAddress() && metadata.Destination.Port == 53 {
		return t.serveTCPDNS(conn)
	}
	upstream, err := t.dial(ctx, "tcp", metadata.Destination)
	if err != nil {
		return err
	}
	defer upstream.Close()
	if !t.track(upstream) {
		return net.ErrClosed
	}
	defer t.connections.Delete(upstream)
	tunipc.Relay(conn, upstream, false)
	return nil
}

func (t *tunIngress) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	defer conn.Close()
	if !t.track(conn) {
		return net.ErrClosed
	}
	defer t.connections.Delete(conn)
	if metadata.Destination.Addr.String() == t.dnsAddress() && metadata.Destination.Port == 53 {
		return t.serveUDPDNS(conn)
	}
	upstream, err := t.dial(ctx, "udp", metadata.Destination)
	if err != nil {
		return err
	}
	defer upstream.Close()
	if !t.track(upstream) {
		return net.ErrClosed
	}
	defer t.connections.Delete(upstream)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer upstream.Close()
		for {
			packet := buf.NewPacket()
			_, err := conn.ReadPacket(packet)
			if err == nil {
				_, err = upstream.Write(packet.Bytes())
			}
			packet.Release()
			if err != nil {
				return
			}
		}
	}()
	for {
		_ = upstream.SetReadDeadline(time.Now().Add(60 * time.Second))
		data := make([]byte, 65535)
		n, err := upstream.Read(data)
		if err != nil {
			break
		}
		if err := conn.WritePacket(buf.As(data[:n]), metadata.Destination); err != nil {
			break
		}
	}
	_ = conn.Close()
	_ = upstream.Close()
	<-done
	return nil
}

func (t *tunIngress) serveTCPDNS(conn net.Conn) error {
	for {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
		var length [2]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			return err
		}
		request := make([]byte, int(binary.BigEndian.Uint16(length[:])))
		if _, err := io.ReadFull(conn, request); err != nil {
			return err
		}
		response, err := t.answerDNS(request)
		if err != nil {
			return err
		}
		binary.BigEndian.PutUint16(length[:], uint16(len(response)))
		if _, err := conn.Write(append(length[:], response...)); err != nil {
			return err
		}
	}
}

func (t *tunIngress) serveUDPDNS(conn N.PacketConn) error {
	for {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		packet := buf.NewPacket()
		target, err := conn.ReadPacket(packet)
		if err != nil {
			packet.Release()
			return err
		}
		response, err := t.answerDNS(packet.Bytes())
		packet.Release()
		if err != nil {
			return err
		}
		if err := conn.WritePacket(buf.As(response), target); err != nil {
			return err
		}
	}
}

func (t *tunIngress) answerDNS(data []byte) ([]byte, error) {
	t.mu.RLock()
	ref := t.spec.Ref
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return nil, net.ErrClosed
	}
	answer, routes, err := t.service.DNS(t.ctx, data)
	if err != nil {
		return nil, err
	}
	tunPolicies.Lock()
	defer tunPolicies.Unlock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.spec.Ref != ref {
		return nil, net.ErrClosed
	}
	for _, prefix := range routes {
		p, err := netip.ParsePrefix(prefix)
		if err != nil || !p.Addr().Is4() || p.Bits() != 32 {
			return nil, errors.New("invalid DNS route")
		}
		err = checkTUNConflicts(t, []string{prefix}, nil)
		if err == nil {
			err = t.routes.add(prefix)
		}
		if err != nil {
			request := new(dns.Msg)
			if e := request.Unpack(data); e != nil {
				return nil, e
			}
			reply := new(dns.Msg)
			reply.SetRcode(request, dns.RcodeServerFailure)
			return reply.Pack()
		}
		claim := tunPolicies.claims[t]
		if !slices.Contains(claim.prefixes, prefix) {
			claim.prefixes = append(claim.prefixes, prefix)
			tunPolicies.claims[t] = claim
		}
	}
	return answer, nil
}

func (t *tunIngress) NewError(_ context.Context, err error) { log.Printf("TUN: %v", err) }

type tunLogger struct{}

var _ logger.Logger = tunLogger{}

func (tunLogger) Trace(args ...any) { log.DebugPrintln(args...) }
func (tunLogger) Debug(args ...any) { log.DebugPrintln(args...) }
func (tunLogger) Info(args ...any)  { log.Println(args...) }
func (tunLogger) Warn(args ...any)  { log.Println(args...) }
func (tunLogger) Error(args ...any) { log.Println(args...) }
func (tunLogger) Fatal(args ...any) { log.Println(args...) }
func (tunLogger) Panic(args ...any) { log.Println(args...) }

type windowsRoutes struct {
	index    int
	tag, dns string
}

// Values travel as JSON on stdin; the script is constant and no profile text
// is interpolated into PowerShell source. All changes name our interface.
func (w *windowsRoutes) run(payload any) (routeResult, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return routeResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", networkScript)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Stdin = strings.NewReader(string(data))
	output, err := command.CombinedOutput()
	if err != nil {
		return routeResult{}, fmt.Errorf("配置 Windows 网络失败: %w: %s", err, output)
	}
	var result routeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return result, fmt.Errorf("解析网络操作结果: %w: %s", err, output)
	}
	if result.Error != "" {
		return result, errors.New(result.Error)
	}
	return result, nil
}

func (w *windowsRoutes) configure() error {
	_, err := w.run(map[string]any{"action": "configure", "index": w.index, "tag": w.dnsTag()})
	return err
}
func (w *windowsRoutes) setDNS(names []string) error {
	sort.Strings(names)
	_, err := w.run(map[string]any{"action": "dns", "names": names, "server": w.dnsAddress(), "tag": w.dnsTag()})
	return err
}

// CleanupStaleTUN runs before connecting, after Wails acquires its single-instance
// lock. Probe mode detects residue without requiring administrator privileges.
func CleanupStaleTUN(probeOnly bool) (bool, error) {
	result, err := (&windowsRoutes{}).run(map[string]any{
		"action": "cleanup", "probe": probeOnly, "tag": nrptTag,
		"name": tunName, "address": netip.MustParsePrefix(tunPrefix).Addr().String(),
	})
	return result.Pending, err
}

const networkScript = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$p = [Console]::In.ReadToEnd() | ConvertFrom-Json
$added = @(); $removed = @(); $failure = ''; $pending = $false
try {
  switch ($p.action) {
    'cleanup' {
      $rules = @(Get-DnsClientNrptRule | Where-Object { $_.Comment -eq $p.tag -or $_.Comment -match '^EasyConnectSuite resource TUN #[0-9]+$' })
      $routes = @(foreach ($adapter in @(Get-NetAdapter -IncludeHidden | Where-Object { $_.Name -eq $p.name -or $_.Name -match '^EasyConnect Suite #[0-9]+$' })) {
        $addresses = @(Get-NetIPAddress | Where-Object { $_.InterfaceIndex -eq $adapter.InterfaceIndex -and $_.AddressFamily -eq 'IPv4' })
        $expected = $p.address
        if ($adapter.Name -match '^EasyConnect Suite #([0-9]+)$') {
          $slot = [int]$Matches[1]
          if ($slot -lt 1 -or $slot -gt 63) { continue }
          $expected = '198.19.128.' + ($slot * 4 + 1)
        }
        if (@($addresses | Where-Object { $_.IPAddress -eq $expected }).Count -gt 0) {
          Get-NetRoute -InterfaceIndex $adapter.InterfaceIndex -AddressFamily IPv4 -PolicyStore ActiveStore |
            Where-Object { $_.Protocol -eq 'NetMgmt' -and $_.RouteMetric -eq 0 -and $_.NextHop -eq '0.0.0.0' -and $_.DestinationPrefix -ne '0.0.0.0/0' }
        }
      })
      $pending = $rules.Count -gt 0 -or $routes.Count -gt 0
      if ($pending -and -not $p.probe) {
        foreach ($rule in $rules) { Remove-DnsClientNrptRule -Name $rule.Name -Force }
        foreach ($route in $routes) {
          $route | Remove-NetRoute -PolicyStore ActiveStore -Confirm:$false
          $removed += $route.DestinationPrefix
        }
        Clear-DnsClientCache
      }
    }
    'configure' {
      Set-NetIPInterface -InterfaceIndex $p.index -AddressFamily IPv4 -AutomaticMetric Disabled -InterfaceMetric 0
      Set-DnsClientServerAddress -InterfaceIndex $p.index -ResetServerAddresses
      Get-DnsClientNrptRule | Where-Object { $_.Comment -eq $p.tag } | ForEach-Object { Remove-DnsClientNrptRule -Name $_.Name -Force }
    }
    'dns' {
      Get-DnsClientNrptRule | Where-Object { $_.Comment -eq $p.tag } | ForEach-Object { Remove-DnsClientNrptRule -Name $_.Name -Force }
      if (@($p.names).Count -gt 0 -and $null -ne $p.names) {
        Add-DnsClientNrptRule -Namespace @($p.names) -NameServers $p.server -Comment $p.tag | Out-Null
      }
      Clear-DnsClientCache
    }
  }
} catch { $failure = $_.Exception.Message }
@{ added = @($added); removed = @($removed); error = $failure; pending = $pending } | ConvertTo-Json -Compress
`
