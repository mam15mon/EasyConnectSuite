//go:build windows

package engine

import (
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNativeRoutesPreserveOwnershipAndPartialFailures(t *testing.T) {
	for _, scenario := range []string{"replace", "add failure", "delete failure", "concurrent add", "concurrent delete"} {
		t.Run(scenario, func(t *testing.T) {
			oldRead, oldAdd, oldRemove := readIPRoutes, addIPRoute, removeIPRoute
			t.Cleanup(func() { readIPRoutes, addIPRoute, removeIPRoute = oldRead, oldAdd, oldRemove })
			makeRow := func(index uint32, prefix string) windows.MibIpForwardRow2 {
				row, err := ipRouteRow(index, prefix)
				if err != nil {
					t.Fatal(err)
				}
				return row
			}
			preexisting := makeRow(42, "10.0.0.0/8")
			owned := makeRow(42, "192.0.2.0/24")
			otherVPN := makeRow(43, "172.31.0.0/16")
			gatewayRoute := makeRow(42, "192.0.2.0/24")
			(*windows.RawSockaddrInet4)(unsafe.Pointer(&gatewayRoute.NextHop)).Addr = [4]byte{192, 0, 2, 1}
			reads, adds, deletes := 0, 0, 0
			readIPRoutes = func() ([]windows.MibIpForwardRow2, error) {
				reads++
				return []windows.MibIpForwardRow2{preexisting, owned, otherVPN, gatewayRoute}, nil
			}
			addIPRoute = func(row *windows.MibIpForwardRow2) error {
				adds++
				if row.InterfaceIndex != 42 || row.Metric != 0 || row.NextHop.Family != windows.AF_INET ||
					(*windows.RawSockaddrInet4)(unsafe.Pointer(&row.NextHop)).Addr != [4]byte{} ||
					row.SitePrefixLength != 0 || row.Protocol != windows.MIB_IPPROTO_NETMGMT {
					t.Fatalf("incorrect route settings: %+v", row)
				}
				if scenario == "add failure" && adds == 2 {
					return windows.ERROR_ACCESS_DENIED
				}
				if scenario == "concurrent add" {
					return windows.ERROR_OBJECT_ALREADY_EXISTS
				}
				return nil
			}
			removeIPRoute = func(row *windows.MibIpForwardRow2) error {
				deletes++
				if *row != owned {
					t.Fatalf("removed a route on another VPN or via a gateway: %+v", row)
				}
				if scenario == "delete failure" {
					return windows.ERROR_ACCESS_DENIED
				}
				if scenario == "concurrent delete" {
					return windows.ERROR_NOT_FOUND
				}
				return nil
			}
			result, err := (&windowsRoutes{index: 42}).Apply([]string{"10.0.0.0/8", "172.31.0.0/16", "203.0.113.0/24"}, []string{"192.0.2.0/24"})
			wantAdded := []string{"172.31.0.0/16", "203.0.113.0/24"}
			wantRemoved := []string{"192.0.2.0/24"}
			if scenario == "add failure" {
				wantAdded, wantRemoved = wantAdded[:1], nil
			}
			if scenario == "delete failure" {
				wantRemoved = nil
			}
			if scenario == "concurrent add" {
				wantAdded = nil
			}
			wantFailure := scenario == "add failure" || scenario == "delete failure"
			if !slices.Equal(result.Added, wantAdded) || !slices.Equal(result.Removed, wantRemoved) ||
				wantFailure != errors.Is(err, windows.ERROR_ACCESS_DENIED) || reads != 1 || adds != 2 {
				t.Fatalf("result=%+v error=%v reads=%d adds=%d deletes=%d", result, err, reads, adds, deletes)
			}
			if scenario == "add failure" && deletes != 0 || scenario != "add failure" && deletes != 1 {
				t.Fatalf("unexpected deletion count: %d", deletes)
			}
		})
	}
}

func TestNativeRoutePrefixEncoding(t *testing.T) {
	for _, prefix := range []string{"10.1.2.3/8", "172.31.108.6/32", tunDNS + "/32"} {
		row, err := ipRouteRow(42, prefix)
		if err != nil {
			t.Fatal(err)
		}
		address := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.DestinationPrefix.Prefix))
		got := netip.PrefixFrom(netip.AddrFrom4(address.Addr), int(row.DestinationPrefix.PrefixLength))
		if got != netip.MustParsePrefix(prefix).Masked() || address.Family != windows.AF_INET {
			t.Fatalf("incorrect prefix encoding: %s", got)
		}
	}
	for _, prefix := range []string{"invalid", "2001:db8::/32"} {
		if _, err := ipRouteRow(42, prefix); err == nil {
			t.Fatalf("accepted invalid IPv4 prefix: %s", prefix)
		}
	}
}

func TestNativeRouteSnapshot(t *testing.T) {
	started := time.Now()
	rows, err := windowsIPRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("empty live routing table")
	}
	t.Logf("read %d IPv4 routes in %s", len(rows), time.Since(started))
}
