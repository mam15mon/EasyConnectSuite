//go:build windows

package engine

import (
	"encoding/json"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// All network cmdlets are replaced so these tests cannot change system state.
const cleanupCmdletMocks = `
$script:dnsRemoved = 0; $script:routeRemoved = 0; $script:flushed = 0
function Get-DnsClientNrptRule {
  [pscustomobject]@{ Name = 'other'; Comment = 'another VPN' }
  if ($p.scenario -ne 'clean' -and $p.scenario -ne 'wrong-address') {
    [pscustomobject]@{ Name = 'ours'; Comment = 'EasyConnectSuite resource TUN' }
  }
}
function Get-NetAdapter {
  param([switch]$IncludeHidden)
  [pscustomobject]@{ Name = 'another VPN'; InterfaceIndex = 43 }
  if ($p.scenario -ne 'clean' -and $p.scenario -ne 'dns-only') {
    [pscustomobject]@{ Name = 'EasyConnect Suite'; InterfaceIndex = 42 }
  }
}
function Get-NetIPAddress {
  [pscustomobject]@{ IPAddress = '198.19.128.1'; InterfaceIndex = 43; AddressFamily = 'IPv4' }
  if ($p.scenario -eq 'no-address') { return }
  $address = '198.19.128.1'
  if ($p.scenario -eq 'wrong-address') { $address = '192.0.2.1' }
  [pscustomobject]@{ IPAddress = $address; InterfaceIndex = 42; AddressFamily = 'IPv4' }
}
function Get-NetRoute {
  param($InterfaceIndex, $AddressFamily, $PolicyStore)
  if ($InterfaceIndex -ne 42 -or $AddressFamily -ne 'IPv4' -or $PolicyStore -ne 'ActiveStore') { throw 'queried another routing table' }
  foreach ($case in @(
    @('10.42.0.0/16', 'NetMgmt', 0, '0.0.0.0'),
    @('0.0.0.0/0', 'NetMgmt', 0, '0.0.0.0'),
    @('192.0.2.0/24', 'NetMgmt', 5, '0.0.0.0'),
    @('198.51.100.0/24', 'Local', 0, '0.0.0.0'),
    @('203.0.113.0/24', 'NetMgmt', 0, '192.0.2.1')
  )) {
    [pscustomobject]@{ DestinationPrefix = $case[0]; Protocol = $case[1]; RouteMetric = $case[2]; NextHop = $case[3]; InterfaceIndex = 42 }
  }
}
function Remove-DnsClientNrptRule {
  param($Name, [switch]$Force)
  if ($Name -ne 'ours' -or -not $Force) { throw 'removed another DNS rule' }
  if ($p.scenario -eq 'failure') { throw 'access denied' }
  $script:dnsRemoved++
}
function Remove-NetRoute {
  param([Parameter(ValueFromPipeline)]$InputObject, $PolicyStore, $Confirm)
  process {
    if ($InputObject.DestinationPrefix -ne '10.42.0.0/16' -or $PolicyStore -ne 'ActiveStore') { throw 'removed another route' }
    $script:routeRemoved++
  }
}
function Clear-DnsClientCache { $script:flushed++ }
`

func TestStaleTUNCleanupScopeAndProbe(t *testing.T) {
	for _, test := range []struct {
		name                        string
		probe, pending              bool
		dnsRemoved, routes, flushed int
		wantError                   bool
	}{
		{"clean", false, false, 0, 0, 0, false},
		{"residue", true, true, 0, 0, 0, false},
		{"residue", false, true, 1, 1, 1, false},
		{"dns-only", false, true, 1, 0, 1, false},
		{"no-address", false, true, 1, 0, 1, false},
		{"wrong-address", false, false, 0, 0, 0, false},
		{"failure", false, true, 0, 0, 0, true},
	} {
		t.Run(test.name+map[bool]string{true: "-probe", false: "-cleanup"}[test.probe], func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{"action": "cleanup", "probe": test.probe, "scenario": test.name, "tag": nrptTag, "name": tunName, "address": "198.19.128.1"})
			script := cleanupCmdletMocks + "\n$result = & {\n" + networkScript + "\n} | ConvertFrom-Json\n" + `
$result | Add-Member -NotePropertyName dnsRemoved -NotePropertyValue $script:dnsRemoved
$result | Add-Member -NotePropertyName routes -NotePropertyValue $script:routeRemoved
$result | Add-Member -NotePropertyName flushed -NotePropertyValue $script:flushed
$result | ConvertTo-Json -Compress
`
			cmd := exec.CommandContext(t.Context(), "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			cmd.Stdin = strings.NewReader(string(payload))
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("mock cleanup failed: %v: %s", err, output)
			}
			var result struct {
				routeResult
				DNSRemoved int `json:"dnsRemoved"`
				Routes     int `json:"routes"`
				Flushed    int `json:"flushed"`
			}
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("invalid cleanup output: %v: %s", err, output)
			}
			if result.Pending != test.pending || result.DNSRemoved != test.dnsRemoved || result.Routes != test.routes || result.Flushed != test.flushed || (result.Error != "") != test.wantError {
				t.Fatalf("unexpected cleanup result: %+v", result)
			}
		})
	}
}

func TestStaleCleanupIncludesAllOwnedTUNSlots(t *testing.T) {
	const mocks = `
$script:dnsRemoved = 0; $script:routesRemoved = 0
function Get-DnsClientNrptRule {
 [pscustomobject]@{ Name='a'; Comment='EasyConnectSuite resource TUN' }
 [pscustomobject]@{ Name='b'; Comment='EasyConnectSuite resource TUN #1' }
 [pscustomobject]@{ Name='other'; Comment='another VPN' }
}
function Get-NetAdapter {
 [pscustomobject]@{ Name='EasyConnect Suite'; InterfaceIndex=42 }
 [pscustomobject]@{ Name='EasyConnect Suite #1'; InterfaceIndex=43 }
 [pscustomobject]@{ Name='EasyConnect Suite #2'; InterfaceIndex=44 }
 [pscustomobject]@{ Name='another VPN'; InterfaceIndex=45 }
}
function Get-NetIPAddress {
 [pscustomobject]@{ InterfaceIndex=42; AddressFamily='IPv4'; IPAddress='198.19.128.1' }
 [pscustomobject]@{ InterfaceIndex=43; AddressFamily='IPv4'; IPAddress='198.19.128.5' }
 [pscustomobject]@{ InterfaceIndex=44; AddressFamily='IPv4'; IPAddress='192.0.2.1' }
 [pscustomobject]@{ InterfaceIndex=45; AddressFamily='IPv4'; IPAddress='198.19.128.9' }
}
function Get-NetRoute {
 param($InterfaceIndex,$AddressFamily,$PolicyStore)
 if ($InterfaceIndex -notin @(42,43)) { throw 'touched wrong adapter' }
 [pscustomobject]@{InterfaceIndex=$InterfaceIndex;DestinationPrefix='10.42.0.0/16';Protocol='NetMgmt';RouteMetric=0;NextHop='0.0.0.0'}
}
function Remove-DnsClientNrptRule {
 param($Name,[switch]$Force)
 if ($Name -notin @('a','b')) { throw 'removed other DNS' }; $script:dnsRemoved++
}
function Remove-NetRoute {
 param([Parameter(ValueFromPipeline)]$InputObject,$PolicyStore,$Confirm)
 process { if ($InputObject.InterfaceIndex -notin @(42,43)) { throw 'removed other route' };$script:routesRemoved++ }
}
function Clear-DnsClientCache {}
`
	for _, probe := range []bool{true, false} {
		payload, _ := json.Marshal(map[string]any{"action": "cleanup", "probe": probe, "tag": nrptTag, "name": tunName, "address": "198.19.128.1"})
		script := mocks + "\n$result = & {\n" + networkScript + "\n} | ConvertFrom-Json\n" + `
$result | Add-Member -NotePropertyName dnsRemoved -NotePropertyValue $script:dnsRemoved
$result | Add-Member -NotePropertyName routesRemoved -NotePropertyValue $script:routesRemoved
$result | ConvertTo-Json -Compress
`
		cmd := exec.CommandContext(t.Context(), "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		cmd.Stdin = strings.NewReader(string(payload))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err, string(output))
		}
		var result struct {
			routeResult
			DNSRemoved    int `json:"dnsRemoved"`
			RoutesRemoved int `json:"routesRemoved"`
		}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatal(err, string(output))
		}
		want := 2
		if probe {
			want = 0
		}
		if !result.Pending || result.Error != "" || result.DNSRemoved != want || result.RoutesRemoved != want {
			t.Fatalf("multi-slot cleanup: %s", output)
		}
	}
}
