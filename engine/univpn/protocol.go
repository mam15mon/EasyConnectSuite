// Package univpn implements the observed password/IPv4-over-TLS UniVPN subset.
// Protocol reference: https://github.com/Bruce-lhq/univpn-connect/blob/main/docs/protocol.md
// The OpenConnect reference implementation is LGPL-2.1-or-later.
// This package does not implement MFA, IPv6, DTLS or firmware negotiation.
package univpn

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
)

var ErrLoginRejected = errors.New("UniVPN 服务器拒绝账号或密码")
var errProtocol = errors.New("无效的 UniVPN 协议数据")

var magic = [5]byte{0xc1, 0x92, 0xa4, 0xd6, 1}

func readFrame(r io.Reader) (byte, []byte, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	if !bytes.Equal(header[:5], magic[:]) {
		return 0, nil, errProtocol
	}
	body := make([]byte, int(binary.BigEndian.Uint16(header[6:])))
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return header[5], body, nil
}

func frame(command byte, body []byte) ([]byte, error) {
	if len(body) > 65535 {
		return nil, errProtocol
	}
	out := append([]byte(nil), magic[:]...)
	out = append(out, command)
	out = binary.BigEndian.AppendUint16(out, uint16(len(body)))
	return append(out, body...), nil
}

func appendTLV(body []byte, kind uint16, value []byte) ([]byte, error) {
	if len(value) > 65535 || len(body)+len(value)+4 > 65535 {
		return nil, errProtocol
	}
	body = binary.BigEndian.AppendUint16(body, kind)
	body = binary.BigEndian.AppendUint16(body, uint16(len(value)))
	return append(body, value...), nil
}

func parseTLVs(body []byte, visit func(uint16, []byte) error) error {
	for len(body) > 0 {
		if len(body) < 4 {
			return errProtocol
		}
		n := int(binary.BigEndian.Uint16(body[2:4]))
		if n > len(body)-4 {
			return errProtocol
		}
		if err := visit(binary.BigEndian.Uint16(body[:2]), body[4:4+n]); err != nil {
			return err
		}
		body = body[4+n:]
	}
	return nil
}

func authValue(value string) []byte {
	escaped := strings.NewReplacer("%", "%25", "&", "%26", "+", "%2B").Replace(value)
	return append([]byte(escaped), 0)
}

func parseToken(body []byte) ([32]byte, error) {
	var token [32]byte
	if len(body) < 8 {
		return token, errProtocol
	}
	if binary.BigEndian.Uint32(body[:4]) != 0 {
		return token, ErrLoginRejected
	}
	found := false
	err := parseTLVs(body[8:], func(kind uint16, value []byte) error {
		if kind == 3 {
			if found || len(value) != len(token) {
				return errProtocol
			}
			copy(token[:], value)
			found = true
		}
		return nil
	})
	if err != nil {
		return token, err
	}
	if !found {
		return token, errProtocol
	}
	return token, nil
}

func parseVIP(body []byte) (netip.Addr, []netip.Prefix, error) {
	var ip netip.Addr
	var routes []netip.Prefix
	if len(body) < 4 || body[0] != 0 {
		return ip, nil, errProtocol
	}
	err := parseTLVs(body[4:], func(kind uint16, value []byte) error {
		switch kind {
		case 1:
			if ip.IsValid() || len(value) != 5 || value[4] > 32 {
				return errProtocol
			}
			ip = netip.AddrFrom4([4]byte(value[:4]))
			if ip.IsUnspecified() || ip.IsMulticast() {
				return errProtocol
			}
		case 6:
			if len(value)%5 != 0 {
				return errProtocol
			}
			for len(value) > 0 {
				if value[4] > 32 {
					return errProtocol
				}
				prefix := netip.PrefixFrom(netip.AddrFrom4([4]byte(value[:4])), int(value[4]))
				if prefix != prefix.Masked() {
					return errProtocol
				}
				routes = append(routes, prefix)
				value = value[5:]
			}
		}
		return nil
	})
	if err != nil || !ip.IsValid() {
		return netip.Addr{}, nil, fmt.Errorf("解析 UniVPN 地址与路由: %w", errProtocol)
	}
	return ip, routes, nil
}

func validIPv4(packet []byte) bool {
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return false
	}
	header := int(packet[0]&15) * 4
	return header >= 20 && header <= len(packet) && int(binary.BigEndian.Uint16(packet[2:4])) == len(packet)
}
