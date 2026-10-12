package topsec

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// This is the observed native PCI-NIC identity path, without calling TopSAP's
// local service or copying its saved identifier. Preserve GetAdaptersInfo order.
func localDeviceCode(profileID string) (string, error) {
	var size uint32
	if err := windows.GetAdaptersInfo(nil, &size); !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
		return "", fmt.Errorf("TopSAP 读取网卡失败: %w", err)
	}
	buf := make([]byte, size)
	adapter := (*windows.IpAdapterInfo)(unsafe.Pointer(&buf[0]))
	if err := windows.GetAdaptersInfo(adapter, &size); err != nil {
		return "", err
	}
	return deviceCodeFromAdapters(adapter, profileID, adapterPNPInstanceID)
}

func adapterPNPInstanceID(name string) (string, error) {
	path := `SYSTEM\CurrentControlSet\Control\Network\{4D36E972-E325-11CE-BFC1-08002BE10318}\` + name + `\Connection`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open_registry: %w", err)
	}
	defer key.Close()
	value, _, err := key.GetStringValue("PnPInstanceId")
	if err != nil {
		return "", fmt.Errorf("read_pnp: %w", err)
	}
	return value, nil
}

func deviceCodeFromAdapters(adapter *windows.IpAdapterInfo, profileID string, readPNP func(string) (string, error)) (string, error) {
	var selected [][]byte
	count := 0
	for a := adapter; a != nil; a = a.Next {
		count++
		name := string(bytes.TrimRight(a.AdapterName[:], "\x00"))
		pnp, err := readPNP(name)
		if err != nil {
			log.Printf("TopSAP pid=%d profile=%q device adapter=%d type=%d mac_bytes=%d skipped=%v", os.Getpid(), profileID, count, a.Type, a.AddressLength, err)
			continue
		}
		pci := strings.Contains(pnp, "PCI")
		accepted := pci && a.AddressLength == 6
		log.Printf("TopSAP pid=%d profile=%q device adapter=%d type=%d pnp_present=%t pci=%t mac_bytes=%d selected=%t", os.Getpid(), profileID, count, a.Type, pnp != "", pci, a.AddressLength, accepted)
		if accepted {
			selected = append(selected, append([]byte(nil), a.Address[:6]...))
		}
	}
	log.Printf("TopSAP pid=%d profile=%q device enumeration complete adapters=%d selected=%d", os.Getpid(), profileID, count, len(selected))
	if selected == nil {
		return "", errors.New("TopSAP 未找到已验证的物理 PCI 网卡标识")
	}
	return deviceCode(selected...)
}
