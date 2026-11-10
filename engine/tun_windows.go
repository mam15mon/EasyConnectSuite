//go:build windows

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"

	"EasyConnectSuite/vpncore/log"
	stun "github.com/mythologyli/sing-tun"
	"golang.org/x/sys/windows"
)

const tunName = "EasyConnect Suite"
const tunPrefix = "198.19.128.1/30"
const nrptTag = "EasyConnectSuite resource TUN"

// sing-tun already embeds Wintun. The ingress converts TCP/UDP flows into
// calls to our existing VPN dialer, so gVisor reconnect and IP updates remain
// shared with SOCKS5. It never changes the system's default route.
func StartTUNIngress(spec TUNSpec, service TUNBackend, progress func(string, string)) (_ *tunIngress, err error) {
	name, prefixText, dnsIP, tag, err := tunIdentity(spec.Slot)
	if err != nil {
		return nil, err
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return nil, fmt.Errorf("请以管理员身份运行 VPN 后启用 TUN")
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &tunIngress{name: name, dns: dnsIP, prefix: prefixText, service: service, ctx: ctx, cancel: cancel}
	defer func() {
		if err != nil {
			err = joinTUNCleanupError(err, t.Close())
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
	backend := &windowsRoutes{index: iface.Index, tag: tag, dns: dnsIP}
	t.backend = backend
	t.routes = newRouteManager(backend)
	progress("配置网卡", "正在配置 TUN 网卡和接口优先级…")
	if err = backend.configure(); err != nil {
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
