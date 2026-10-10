// Package topsec implements the observed TopSAP standard IPv4 protocol in Go.
package topsec

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
)

var ErrLoginRejected = errors.New("TopSAP 认证被服务器拒绝")
var errProtocol = errors.New("TopSAP 报文格式或模式不受支持")

type AuthenticationError struct{ Code int }

func (e *AuthenticationError) Error() string {
	return fmt.Sprintf("TopSAP 服务器拒绝认证（Ret=%d）", e.Code)
}
func (e *AuthenticationError) Unwrap() error { return ErrLoginRejected }

const authAgent = "VONE-AUTH,WINDOWS,NULL,TOPCLINET,NULL,CLIENT_TYPE=ng2.2,VERSION=3.6.6.18.64.1"
const connectPrelude = "CONNECT 127.0.0.1:4430 HTTP/1.1\r\nhost:127.0.0.1\r\nUser-Agent: VONE-NA,WINDOWS,NULL,TOPCLINET,NULL\r\nProxy-Connection: Keep-Alive\r\n\r\n"
const maxResponse = 4 << 20

// The native NIC identifier uses three uppercase hex digits per byte.
// MD5 here is a wire-format device identifier, not password protection.
func deviceCode(macs ...[]byte) (string, error) {
	if len(macs) == 0 {
		return "", errors.New("TopSAP 未找到已验证的物理 PCI 网卡标识")
	}
	var text strings.Builder
	for _, mac := range macs {
		if len(mac) != 6 {
			return "", errors.New("TopSAP 需要六字节物理网卡标识")
		}
		fmt.Fprintf(&text, "%03X-%03X-%03X-%03X-%03X-%03X", mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
	}
	digest := md5.Sum([]byte(text.String()))
	return base64.StdEncoding.EncodeToString(digest[:]), nil
}

// auth_param has JavaScript-style identifier keys. Only quote keys; never
// evaluate gateway content or alter quoted values/escape sequences.
func decodeObject(data []byte, target any) error {
	var out bytes.Buffer
	quoted, escaped := false, false
	for i := 0; i < len(data); {
		c := data[i]
		if quoted {
			out.WriteByte(c)
			i++
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			out.WriteByte(c)
			i++
			continue
		}
		if c == '{' || c == ',' {
			out.WriteByte(c)
			i++
			for i < len(data) && strings.ContainsRune(" \r\n\t", rune(data[i])) {
				out.WriteByte(data[i])
				i++
			}
			start := i
			for i < len(data) && (data[i] >= 'a' && data[i] <= 'z' || data[i] >= 'A' && data[i] <= 'Z' || data[i] == '_' || i > start && data[i] >= '0' && data[i] <= '9') {
				i++
			}
			end := i
			for end < len(data) && strings.ContainsRune(" \r\n\t", rune(data[end])) {
				end++
			}
			if i > start && end < len(data) && data[end] == ':' {
				out.WriteByte('"')
				out.Write(data[start:i])
				out.WriteByte('"')
			} else {
				out.Write(data[start:i])
			}
			continue
		}
		out.WriteByte(c)
		i++
	}
	if err := json.Unmarshal(out.Bytes(), target); err != nil {
		return errProtocol
	}
	return nil
}

type authParameters struct {
	Password     string `json:"pswd"`
	CheckClient  string `json:"check_client_before_login"`
	AuthProtocol string `json:"auth_protocol"`
	DataProtocol string `json:"data_protocol"`
	TwoWay       string `json:"twoway_auth_switch"`
}
type userParameters struct {
	Token     string `json:"topafasfasfassession_id"`
	NetAccess string `json:"netacc"`
	Algorithm string `json:"alg"`
	Protocol  string `json:"protocal"`
	Compress  string `json:"compress"`
}
type netParameters struct {
	IP   string `json:"vip_v4"`
	DNS1 string `json:"dns1_v4"`
	DNS2 string `json:"dns2_v4"`
}
type resource struct {
	Module   string `json:"module"`
	Protocol string `json:"protocol"`
	IP       string `json:"ip"`
	Mask     string `json:"mask"`
	Domain   string `json:"domain"`
	Port     string `json:"port"`
}
type reply struct {
	Ret       *int           `json:"Ret"`
	User      userParameters `json:"user_param"`
	Net       netParameters  `json:"net_param"`
	Resources []resource     `json:"res_list"`
	DNS       struct {
		List    string `json:"dns_list"`
		Domains string `json:"domain_list"`
	} `json:"dns_info"`
	Security struct {
		Count int `json:"num"`
	} `json:"sec_rule"`
}

func (r *reply) check() error {
	if r.Ret == nil {
		return errProtocol
	}
	if *r.Ret != 0 {
		return &AuthenticationError{Code: *r.Ret}
	}
	return nil
}
func parseResources(r reply) ([]netip.Prefix, error) {
	if r.Security.Count != 0 {
		return nil, errors.New("TopSAP 客户端安全检查暂不支持")
	}
	if r.DNS.List != "" || r.DNS.Domains != "" {
		return nil, errors.New("TopSAP 此网关的域名/DNS 资源格式尚未验证")
	}
	if r.Resources == nil {
		return nil, errors.New("TopSAP 未返回完整资源列表")
	}
	var routes []netip.Prefix
	for _, row := range r.Resources {
		if row.Module != "na" {
			continue
		}
		if row.Protocol != "ip" || row.Domain != "" || row.Port != "" {
			return nil, errors.New("TopSAP 该网络资源类型暂不支持")
		}
		ip, err := netip.ParseAddr(row.IP)
		mask := net.ParseIP(row.Mask).To4()
		if err != nil || !ip.Is4() || mask == nil {
			return nil, errProtocol
		}
		ones, bits := net.IPMask(mask).Size()
		if bits != 32 {
			return nil, errProtocol
		}
		routes = append(routes, netip.PrefixFrom(ip, ones).Masked())
	}
	return routes, nil
}

func frame(command uint32, body []byte) []byte {
	packet := make([]byte, 16+len(body))
	binary.BigEndian.PutUint32(packet[4:], command)
	binary.BigEndian.PutUint32(packet[8:], uint32(len(body)))
	binary.BigEndian.PutUint32(packet[12:], 1)
	copy(packet[16:], body)
	return packet
}
func readFrame(r io.Reader) (uint32, []byte, error) {
	var head [16]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	first, command, size, last := binary.BigEndian.Uint32(head[:]), binary.BigEndian.Uint32(head[4:]), binary.BigEndian.Uint32(head[8:]), binary.BigEndian.Uint32(head[12:])
	if size > 65535 {
		return 0, nil, errProtocol
	}
	if command == 2 {
		if first != 0 || size != 0 || last != 1 {
			return 0, nil, errProtocol
		}
	} else if command == 15 {
		if first != 0xAABB || last != 0xEEFF0800 {
			return 0, nil, errProtocol
		}
	} else {
		return 0, nil, errProtocol
	}
	body := make([]byte, int(size))
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	if command == 15 && !validIPv4(body) {
		return 0, nil, errProtocol
	}
	return command, body, nil
}
func validIPv4(b []byte) bool {
	return len(b) >= 20 && b[0]>>4 == 4 && int(b[0]&15)*4 >= 20 && int(b[0]&15)*4 <= len(b) && int(binary.BigEndian.Uint16(b[2:])) == len(b)
}
