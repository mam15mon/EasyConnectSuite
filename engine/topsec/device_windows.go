package topsec

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// This is the observed native PCI-NIC identity path, without calling TopSAP's
// local service or copying its saved identifier. Preserve GetAdaptersInfo order.
func localDeviceCode() (string, error) {
	var size uint32
	if err := windows.GetAdaptersInfo(nil, &size); !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
		return "", fmt.Errorf("TopSAP 读取网卡失败: %w", err)
	}
	buf := make([]byte, size)
	adapter := (*windows.IpAdapterInfo)(unsafe.Pointer(&buf[0]))
	if err := windows.GetAdaptersInfo(adapter, &size); err != nil {
		return "", err
	}
	var selected []byte
	for a := adapter; a != nil; a = a.Next {
		name := string(bytes.TrimRight(a.AdapterName[:], "\x00"))
		path := `SYSTEM\CurrentControlSet\Control\Network\{4D36E972-E325-11CE-BFC1-08002BE10318}\` + name + `\Connection`
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		pnp, _, err := key.GetStringValue("PnPInstanceId")
		_ = key.Close()
		if err == nil && strings.Contains(pnp, "PCI") && a.AddressLength == 6 {
			selected = append([]byte(nil), a.Address[:6]...)
		}
	}
	if selected == nil {
		return "", errors.New("TopSAP 未找到已验证的物理 PCI 网卡标识")
	}
	return deviceCode(selected)
}
