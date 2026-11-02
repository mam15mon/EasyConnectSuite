//go:build windows

package engine

import (
	"fmt"
)

func tunIdentity(slot int) (name, prefix, dns, tag string, err error) {
	if slot < 0 || slot > 63 {
		return "", "", "", "", fmt.Errorf("最多支持 64 条同时连接")
	}
	if slot == 0 {
		return tunName, tunPrefix, tunDNS, nrptTag, nil
	}
	return fmt.Sprintf("%s #%d", tunName, slot), fmt.Sprintf("198.19.128.%d/30", slot*4+1), fmt.Sprintf("198.19.128.%d", slot*4+2), fmt.Sprintf("%s #%d", nrptTag, slot), nil
}
func (t *tunIngress) dnsAddress() string {
	if t.dns == "" {
		return tunDNS
	}
	return t.dns
}
func (w *windowsRoutes) dnsAddress() string {
	if w.dns == "" {
		return tunDNS
	}
	return w.dns
}
func (w *windowsRoutes) dnsTag() string {
	if w.tag == "" {
		return nrptTag
	}
	return w.tag
}
