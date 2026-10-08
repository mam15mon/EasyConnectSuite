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
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/miekg/dns"
	stun "github.com/mythologyli/sing-tun"
	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/log"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"golang.org/x/sys/windows"
)

const tunName = "EasyConnect Suite"
const tunPrefix = "198.18.255.1/30"
const tunDNS = "198.18.255.2"
const nrptTag = "EasyConnectSuite resource TUN"

type tunIngress struct {
	mu          sync.RWMutex
	device      stun.Tun
	stack       stun.Stack
	router      *policyRouter
	policy      *connectionPolicy
	routes      *routeManager
	backend     *windowsRoutes
	bindings    map[netip.Addr][]client.DomainResource
	ctx         context.Context
	cancel      context.CancelFunc
	closeOnce   sync.Once
	closeErr    error
	closed      bool
	connections sync.Map
}

// sing-tun already embeds Wintun. The ingress converts TCP/UDP flows into
// calls to our existing VPN dialer, so gVisor reconnect and IP updates remain
// shared with SOCKS5. It never changes the system's default route.
func startTUN(router *policyRouter, policy *connectionPolicy) (_ *tunIngress, err error) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return nil, fmt.Errorf("请以管理员身份运行 EasyConnectGUI 后启用 TUN")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &tunIngress{router: router, policy: policy, bindings: make(map[netip.Addr][]client.DomainResource), ctx: ctx, cancel: cancel}
	defer func() {
		if err != nil {
			_ = t.Close()
		}
	}()
	prefix := netip.MustParsePrefix(tunPrefix)
	t.device, err = stun.New(stun.Options{Name: tunName, Inet4Address: []netip.Prefix{prefix}, MTU: 1400, AutoRoute: false, StrictRoute: false, Logger: tunLogger{}})
	if err != nil {
		return nil, err
	}
	iface, err := net.InterfaceByName(tunName)
	if err != nil {
		return nil, err
	}
	t.backend = &windowsRoutes{index: iface.Index}
	t.routes = newRouteManager(t.backend)
	if err = t.backend.configure(); err != nil {
		return nil, err
	}
	t.stack, err = stun.NewStack("system", stun.StackOptions{Context: ctx, Tun: t.device, Name: tunName, MTU: 1400, Inet4Address: []netip.Prefix{prefix}, UDPTimeout: 60, Handler: t, Logger: tunLogger{}})
	if err != nil {
		return nil, err
	}
	if err = t.stack.Start(); err != nil {
		return nil, err
	}
	if err = t.Update(policy); err != nil {
		return nil, err
	}
	log.Printf("TUN ready: %s; route metric 0, interface metric 0; DNS %s", tunName, tunDNS)
	return t, nil
}

func (t *tunIngress) Update(policy *connectionPolicy) error {
	prefixes, err := resourcePrefixes(policy.ips)
	if err != nil {
		return err
	}
	// A /32 for our virtual DNS beats broad Clash/aTrust routes, without
	// changing the specificity of any server resource route.
	prefixes = append(prefixes, tunDNS+"/32")
	for _, prefix := range prefixes {
		if prefix == "0.0.0.0/0" {
			return fmt.Errorf("服务器下发了默认路由；资源 TUN 不接管全局默认路由")
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return errors.New("TUN closed")
	}
	if err := t.routes.replace(prefixes); err != nil {
		return err
	}
	if err := t.backend.setDNS(dnsNamespaces(policy)); err != nil {
		return err
	}
	t.bindings = make(map[netip.Addr][]client.DomainResource)
	t.policy = policy
	return nil
}

func (t *tunIngress) Close() error {
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		t.mu.Unlock()
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
	policy := t.policy
	resources := append([]client.DomainResource(nil), t.bindings[target.Addr]...)
	closed := t.closed
	t.mu.RUnlock()
	if closed {
		return nil, errors.New("TUN closed")
	}
	if len(resources) > 0 {
		ctx = context.WithValue(ctx, resolve.ContextKeyDomainResource, resources)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return policy.tunDialer.DialIPPort(ctx, network, target.String())
}

func (t *tunIngress) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	defer conn.Close()
	if !t.track(conn) {
		return net.ErrClosed
	}
	defer t.connections.Delete(conn)
	if metadata.Destination.Addr.String() == tunDNS && metadata.Destination.Port == 53 {
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
	done := make(chan struct{})
	go func() { _, _ = io.Copy(upstream, conn); _ = upstream.Close(); close(done) }()
	_, err = io.Copy(conn, upstream)
	_ = conn.Close()
	_ = upstream.Close()
	<-done
	return err
}

func (t *tunIngress) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	defer conn.Close()
	if !t.track(conn) {
		return net.ErrClosed
	}
	defer t.connections.Delete(conn)
	if metadata.Destination.Addr.String() == tunDNS && metadata.Destination.Port == 53 {
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
	request := new(dns.Msg)
	if err := request.Unpack(data); err != nil {
		return nil, err
	}
	response := new(dns.Msg)
	response.SetReply(request)
	response.RecursionAvailable = true
	if len(request.Question) != 1 {
		response.Rcode = dns.RcodeFormatError
		return response.Pack()
	}
	question := request.Question[0]
	t.mu.RLock()
	policy := t.policy
	t.mu.RUnlock()
	ctx, cancel := context.WithTimeout(t.ctx, 10*time.Second)
	defer cancel()
	if question.Qtype != dns.TypeA && question.Qtype != dns.TypeAAAA {
		if net.ParseIP(policy.dnsName) == nil || !policy.useRemoteDNS {
			response.Rcode = dns.RcodeNotImplemented
			return response.Pack()
		}
		upstream, err := policy.stack.DialUDP(ctx, &net.UDPAddr{IP: net.ParseIP(policy.dnsName), Port: 53})
		if err != nil {
			response.Rcode = dns.RcodeServerFailure
			return response.Pack()
		}
		defer upstream.Close()
		_ = upstream.SetDeadline(time.Now().Add(10 * time.Second))
		dnsClient := &dns.Client{Timeout: 10 * time.Second}
		answer, _, err := dnsClient.ExchangeWithConn(request, &dns.Conn{Conn: upstream})
		if err != nil {
			response.Rcode = dns.RcodeServerFailure
			return response.Pack()
		}
		return answer.Pack()
	}
	resolvedCtx, ip, err := policy.resolver.Resolve(ctx, strings.TrimSuffix(question.Name, "."))
	if err != nil {
		response.Rcode = dns.RcodeServerFailure
		return response.Pack()
	}
	if ip.To4() == nil {
		return response.Pack()
	} // Windows TUN ingress is IPv4 only.
	if question.Qtype == dns.TypeAAAA {
		return response.Pack()
	}
	if resources, ok := resolvedCtx.Value(resolve.ContextKeyDomainResource).([]client.DomainResource); ok && len(resources) > 0 {
		address, _ := netip.AddrFromSlice(ip.To4())
		t.mu.Lock()
		if t.closed || t.policy != policy {
			t.mu.Unlock()
			response.Rcode = dns.RcodeServerFailure
			return response.Pack()
		}
		if !ipHasResourceRoute(policy.ips, ip) {
			err = t.routes.add(address.String() + "/32")
		}
		if err == nil {
			existing := t.bindings[address]
			seen := make(map[client.DomainResource]bool)
			for _, resource := range existing {
				seen[resource] = true
			}
			for _, resource := range resources {
				if !seen[resource] {
					existing = append(existing, resource)
					seen[resource] = true
				}
			}
			t.bindings[address] = existing
		}
		t.mu.Unlock()
		if err != nil {
			response.Rcode = dns.RcodeServerFailure
			return response.Pack()
		}
	}
	response.Answer = append(response.Answer, &dns.A{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30}, A: ip.To4()})
	return response.Pack()
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

type windowsRoutes struct{ index int }

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

func (w *windowsRoutes) Apply(add, remove []string) (routeResult, error) {
	if len(add) == 0 && len(remove) == 0 {
		return routeResult{}, nil
	}
	return w.run(map[string]any{"action": "routes", "index": w.index, "add": add, "remove": remove})
}
func (w *windowsRoutes) configure() error {
	_, err := w.run(map[string]any{"action": "configure", "index": w.index, "tag": nrptTag})
	return err
}
func (w *windowsRoutes) setDNS(names []string) error {
	sort.Strings(names)
	_, err := w.run(map[string]any{"action": "dns", "names": names, "server": tunDNS, "tag": nrptTag})
	return err
}

const networkScript = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$p = [Console]::In.ReadToEnd() | ConvertFrom-Json
$added = @(); $removed = @(); $failure = ''
try {
  switch ($p.action) {
    'configure' {
      Set-NetIPInterface -InterfaceIndex $p.index -AddressFamily IPv4 -AutomaticMetric Disabled -InterfaceMetric 0
      Set-DnsClientServerAddress -InterfaceIndex $p.index -ResetServerAddresses
      Get-DnsClientNrptRule | Where-Object { $_.Comment -eq $p.tag } | ForEach-Object { Remove-DnsClientNrptRule -Name $_.Name -Force }
    }
    'routes' {
      foreach ($prefix in $p.add) {
        $existing = @(Get-NetRoute -InterfaceIndex $p.index -DestinationPrefix $prefix -PolicyStore ActiveStore -ErrorAction SilentlyContinue)
        if ($existing.Count -eq 0) {
          New-NetRoute -InterfaceIndex $p.index -DestinationPrefix $prefix -NextHop '0.0.0.0' -RouteMetric 0 -PolicyStore ActiveStore | Out-Null
          $added += $prefix
        }
      }
      foreach ($prefix in $p.remove) {
        Get-NetRoute -InterfaceIndex $p.index -DestinationPrefix $prefix -NextHop '0.0.0.0' -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false
        $removed += $prefix
      }
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
@{ added = @($added); removed = @($removed); error = $failure } | ConvertTo-Json -Compress
`
