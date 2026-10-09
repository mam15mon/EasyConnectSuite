//go:build windows

package engine

import (
	"errors"
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ipHelper          = windows.NewLazySystemDLL("iphlpapi.dll")
	initializeIPRoute = ipHelper.NewProc("InitializeIpForwardEntry")
	createIPRoute     = ipHelper.NewProc("CreateIpForwardEntry2")
	deleteIPRoute     = ipHelper.NewProc("DeleteIpForwardEntry2")
	readIPRoutes      = windowsIPRoutes
	addIPRoute        = func(row *windows.MibIpForwardRow2) error {
		code, _, _ := createIPRoute.Call(uintptr(unsafe.Pointer(row)))
		if code != 0 {
			return windows.Errno(code)
		}
		return nil
	}
	removeIPRoute = func(row *windows.MibIpForwardRow2) error {
		code, _, _ := deleteIPRoute.Call(uintptr(unsafe.Pointer(row)))
		if code != 0 {
			return windows.Errno(code)
		}
		return nil
	}
)

func windowsIPRoutes() ([]windows.MibIpForwardRow2, error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	return append([]windows.MibIpForwardRow2(nil), table.Rows()...), nil
}

func ipRouteRow(index uint32, prefix string) (windows.MibIpForwardRow2, error) {
	var row windows.MibIpForwardRow2
	p, err := netip.ParsePrefix(prefix)
	if err != nil || !p.Addr().Is4() {
		return row, fmt.Errorf("无效的 IPv4 路由: %s", prefix)
	}
	initializeIPRoute.Call(uintptr(unsafe.Pointer(&row)))
	row.InterfaceIndex = index
	address := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.DestinationPrefix.Prefix))
	address.Family = windows.AF_INET
	address.Addr = p.Masked().Addr().As4()
	row.DestinationPrefix.PrefixLength = uint8(p.Bits())
	row.NextHop = windows.RawSockaddrInet{Family: windows.AF_INET}
	row.SitePrefixLength = 0
	row.Metric = 0
	row.Protocol = windows.MIB_IPPROTO_NETMGMT
	return row, nil
}

func (w *windowsRoutes) Apply(add, remove []string) (routeResult, error) {
	var result routeResult
	if len(add) == 0 && len(remove) == 0 {
		return result, nil
	}
	rows, err := readIPRoutes()
	if err != nil {
		return result, fmt.Errorf("读取 Windows 路由失败: %w", err)
	}
	existing := make(map[string][]windows.MibIpForwardRow2)
	for _, row := range rows {
		if row.InterfaceIndex != uint32(w.index) || row.DestinationPrefix.Prefix.Family != windows.AF_INET {
			continue
		}
		address := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.DestinationPrefix.Prefix)).Addr
		prefix := netip.PrefixFrom(netip.AddrFrom4(address), int(row.DestinationPrefix.PrefixLength)).String()
		existing[prefix] = append(existing[prefix], row)
	}
	for _, prefix := range add {
		if len(existing[prefix]) != 0 {
			continue
		}
		row, err := ipRouteRow(uint32(w.index), prefix)
		if err != nil {
			return result, err
		}
		if err := addIPRoute(&row); err != nil {
			// A route created by another process after the snapshot is not ours.
			if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
				continue
			}
			return result, fmt.Errorf("添加路由 %s 失败: %w", prefix, err)
		}
		result.Added = append(result.Added, prefix)
		existing[prefix] = []windows.MibIpForwardRow2{row}
	}
	for _, prefix := range remove {
		for _, row := range existing[prefix] {
			nextHop := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.NextHop))
			if nextHop.Family != windows.AF_INET || nextHop.Addr != [4]byte{} {
				continue
			}
			if err := removeIPRoute(&row); err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
				return result, fmt.Errorf("删除路由 %s 失败: %w", prefix, err)
			}
		}
		result.Removed = append(result.Removed, prefix)
	}
	return result, nil
}
