//go:build windows

package engine

import (
	"fmt"
	"strings"
	"sync"
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

type tunClaim struct{ prefixes, names []string }

var tunPolicies = struct {
	sync.Mutex
	claims map[*tunIngress]tunClaim
}{claims: make(map[*tunIngress]tunClaim)}

func checkTUNConflicts(owner *tunIngress, prefixes, names []string) error {
	for t, claim := range tunPolicies.claims {
		if t == owner {
			continue
		}
		for _, p := range prefixes {
			for _, q := range claim.prefixes {
				if p == q {
					return fmt.Errorf("TUN 资源 %s 已由另一条连接使用，请关闭其中一条的 TUN 或使用各自代理", p)
				}
			}
		}
		for _, name := range names {
			for _, other := range claim.names {
				a, b := strings.TrimPrefix(strings.ToLower(name), "."), strings.TrimPrefix(strings.ToLower(other), ".")
				if a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a) {
					return fmt.Errorf("TUN DNS 区域 %s 与另一条连接冲突", name)
				}
			}
		}
	}
	return nil
}
