package topsec

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	Server, Username, Password string
	TLSConfig                  *tls.Config
	DialContext                func(context.Context, string, string) (net.Conn, error)
}

// Session reads and writes complete IPv4 packets. TCP and UDP are supplied by
// the existing gVisor stack; this transport does not install routes or DNS.
type Session struct {
	id               uint64
	rxBytes, txBytes atomic.Uint64
	conn             *tls.Conn
	http             *http.Client
	transport        *http.Transport
	server           string
	ip               netip.Addr
	routes           []netip.Prefix
	dns              []string
	cancel           context.CancelFunc
	stopWatch        func() bool
	readMu, writeMu  sync.Mutex
	closeOnce        sync.Once
}

var nextSessionID atomic.Uint64

func Dial(ctx context.Context, opts Options) (*Session, error) {
	code, err := localDeviceCode()
	if err != nil {
		return nil, err
	}
	return dial(ctx, opts, code)
}

// dial's explicit identity lets tests use synthetic hardware without persisting
// a real machine identifier or exposing a product configuration override.
func dial(ctx context.Context, opts Options, code string) (_ *Session, err error) {
	host, _, err := net.SplitHostPort(opts.Server)
	if err != nil || host == "" || opts.Username == "" || opts.Password == "" || strings.ContainsRune(opts.Username+opts.Password, 0) {
		return nil, errors.New("TopSAP 服务器、账号或密码配置无效")
	}
	setupCtx, stopSetup := context.WithTimeout(ctx, 30*time.Second)
	defer stopSetup()
	sessionCtx, cancel := context.WithCancel(ctx)
	cfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if opts.TLSConfig != nil {
		cfg = opts.TLSConfig.Clone()
		if cfg.ServerName == "" {
			cfg.ServerName = host
		}
		if cfg.MinVersion < tls.VersionTLS12 {
			cfg.MinVersion = tls.VersionTLS12
		}
	}
	dialer := opts.DialContext
	if dialer == nil {
		dialer = (&net.Dialer{}).DialContext
	}
	transport := &http.Transport{Proxy: nil, DialContext: dialer, TLSClientConfig: cfg, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 64 << 10, ForceAttemptHTTP2: false}
	jar, _ := cookiejar.New(nil)
	s := &Session{id: nextSessionID.Add(1), transport: transport, server: "https://" + opts.Server, cancel: cancel}
	stage := "获取认证参数"
	setStage := func(name string) {
		stage = name
		log.Printf("TopSAP [%d] stage=%s", s.id, stage)
	}
	startedAt := time.Now()
	log.Printf("TopSAP [%d] start gateway=%s certificate_verify=%t", s.id, opts.Server, !cfg.InsecureSkipVerify)
	s.http = &http.Client{Transport: transport, Jar: jar, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("TopSAP 浏览器重定向认证暂不支持")
	}}
	succeeded := false
	authenticated := false
	defer func() {
		if !succeeded {
			err = fmt.Errorf("TopSAP %s: %w", stage, err)
			log.Printf("TopSAP [%d] setup failed elapsed=%s: %v", s.id, time.Since(startedAt).Round(time.Millisecond), err)
			cancel()
			if s.conn != nil {
				_ = s.conn.Close()
			}
			if s.stopWatch != nil {
				s.stopWatch()
			}
			if authenticated {
				s.logout()
			}
			transport.CloseIdleConnections()
		}
	}()
	var parameters authParameters
	setStage("获取认证参数")
	if _, err := s.post(setupCtx, "auth_param", nil, &parameters); err != nil {
		return nil, err
	}
	if parameters.Password != "on" || parameters.AuthProtocol != "0" || parameters.DataProtocol != "0" || parameters.CheckClient == "on" || parameters.TwoWay == "on" {
		return nil, errors.New("TopSAP 该网关的认证/国密/客户端安全检查模式暂不支持")
	}
	setStage("账号密码认证")
	form := url.Values{"U": {base64.StdEncoding.EncodeToString([]byte(opts.Username))}, "P": {base64.StdEncoding.EncodeToString([]byte(opts.Password))}, "HASH": {""}, "S_CODE": {code}}
	var login reply
	key, err := s.post(setupCtx, "user_pswd", form, &login)
	if err != nil {
		return nil, err
	}
	if err = login.check(); err != nil {
		return nil, err
	}
	authenticated = true
	log.Printf("TopSAP [%d] authentication accepted Ret=0", s.id)
	if login.User.NetAccess != "on" || login.User.Algorithm != "standard" || login.User.Protocol != "standard" || login.User.Compress != "off" {
		return nil, errors.New("TopSAP 数据隧道模式暂不支持")
	}
	token := login.User.Token
	if len(token) == 0 || len(token) >= 256 || strings.ContainsRune(token, 0) {
		return nil, errProtocol
	}
	var resources reply
	setStage("获取完整资源 TYPE=all")
	if _, err = s.post(setupCtx, "get_config_info", url.Values{"HASH": {""}, "TYPE": {"all"}}, &resources); err != nil {
		return nil, err
	}
	if err = resources.check(); err != nil {
		return nil, err
	}
	log.Printf("TopSAP [%d] config consistency token_present=%t token_matches_login=%t vip_present=%t vip_matches_login=%t", s.id, resources.User.Token != "", resources.User.Token == login.User.Token, resources.Net.IP != "", resources.Net.IP == login.Net.IP)
	setStage("解析服务器资源")
	s.routes, err = parseResources(resources)
	if err != nil {
		return nil, err
	}
	// TYPE=all may assign a new virtual address after password authentication.
	// The native client configures its data stack from this complete response.
	s.ip, err = netip.ParseAddr(resources.Net.IP)
	if err != nil || !s.ip.Is4() || s.ip.IsUnspecified() {
		return nil, fmt.Errorf("TopSAP 完整配置中的虚拟 IPv4 地址无效: %w", errProtocol)
	}
	log.Printf("TopSAP [%d] data configuration source=full_config vip_changed=%t", s.id, resources.Net.IP != login.Net.IP)
	for _, text := range []string{resources.Net.DNS1, resources.Net.DNS2} {
		if text == "" || text == "0.0.0.0" {
			continue
		}
		addr, err := netip.ParseAddr(text)
		if err != nil || !addr.Is4() {
			return nil, errProtocol
		}
		s.dns = append(s.dns, addr.String())
	}
	log.Printf("TopSAP [%d] resources ready routes=%d dns_servers=%d", s.id, len(s.routes), len(s.dns))
	setStage("连接数据通道")
	raw, err := dialer(setupCtx, "tcp", opts.Server)
	if err != nil {
		return nil, err
	}
	s.conn = tls.Client(raw, cfg)
	stopDial := context.AfterFunc(setupCtx, func() { _ = raw.Close() })
	defer stopDial()
	deadline, _ := setupCtx.Deadline()
	if err = s.conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	setStage("数据通道 TLS 握手")
	if err = s.conn.HandshakeContext(setupCtx); err != nil {
		return nil, err
	}
	state := s.conn.ConnectionState()
	setStage("核对认证与数据通道公钥")
	if len(state.PeerCertificates) == 0 || len(key) == 0 || !bytes.Equal(key, state.PeerCertificates[0].RawSubjectPublicKeyInfo) {
		return nil, errors.New("TopSAP 认证与数据通道公钥不一致")
	}
	setStage("发送 CONNECT 和隧道初始化")
	if err = writeAll(s.conn, []byte(connectPrelude)); err != nil {
		return nil, err
	}
	setup := make([]byte, 328)
	setup[3] = 4
	copy(setup[72:], token)
	if err = writeAll(s.conn, frame(1, setup)); err != nil {
		return nil, err
	}
	setStage("等待隧道确认 ACK")
	command, _, err := readFrame(s.conn)
	if err != nil {
		return nil, err
	}
	if command != 2 {
		return nil, errProtocol
	}
	setStage("发送初始控制命令")
	if err = writeAll(s.conn, frame(7, make([]byte, 8))); err != nil {
		return nil, err
	}
	if err = s.conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	s.stopWatch = context.AfterFunc(sessionCtx, func() { _ = s.conn.Close(); transport.CloseIdleConnections() })
	if err = setupCtx.Err(); err != nil {
		return nil, err
	}
	succeeded = true
	log.Printf("TopSAP [%d] tunnel ready elapsed=%s", s.id, time.Since(startedAt).Round(time.Millisecond))
	return s, nil
}

func (s *Session) post(ctx context.Context, path string, form url.Values, result any) ([]byte, error) {
	startedAt := time.Now()
	log.Printf("TopSAP [%d] HTTP POST endpoint=%s", s.id, path)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.server+"/vone/login/"+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", authAgent)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.http.Do(request)
	if err != nil {
		log.Printf("TopSAP [%d] HTTP failed endpoint=%s elapsed=%s: %v", s.id, path, time.Since(startedAt).Round(time.Millisecond), err)
		return nil, err
	}
	log.Printf("TopSAP [%d] HTTP response endpoint=%s status=%d elapsed=%s", s.id, path, response.StatusCode, time.Since(startedAt).Round(time.Millisecond))
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, &AuthenticationError{Code: response.StatusCode}
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TopSAP %s HTTP 状态 %d", path, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponse {
		return nil, errProtocol
	}
	if result != nil {
		if err = decodeObject(data, result); err != nil {
			return nil, err
		}
	}
	if r, ok := result.(*reply); ok {
		if r.Ret == nil {
			log.Printf("TopSAP [%d] response endpoint=%s bytes=%d Ret=missing resources_present=%t resources=%d", s.id, path, len(data), r.Resources != nil, len(r.Resources))
		} else {
			log.Printf("TopSAP [%d] response endpoint=%s bytes=%d Ret=%d resources_present=%t resources=%d", s.id, path, len(data), *r.Ret, r.Resources != nil, len(r.Resources))
		}
	}
	if response.TLS == nil || len(response.TLS.PeerCertificates) == 0 {
		return nil, errProtocol
	}
	return append([]byte(nil), response.TLS.PeerCertificates[0].RawSubjectPublicKeyInfo...), nil
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) != 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func (s *Session) IP() netip.Addr         { return s.ip }
func (s *Session) Routes() []netip.Prefix { return append([]netip.Prefix(nil), s.routes...) }
func (s *Session) DNSServers() []string   { return append([]string(nil), s.dns...) }
func (s *Session) Read(b []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	command, body, err := readFrame(s.conn)
	if err != nil {
		log.Printf("TopSAP [%d] tunnel read ended: %v", s.id, err)
		return 0, err
	}
	if command != 15 {
		log.Printf("TopSAP [%d] unexpected data command=%d", s.id, command)
		return 0, errProtocol
	}
	if len(b) < len(body) {
		return 0, io.ErrShortBuffer
	}
	if s.rxBytes.Add(uint64(len(body))) == uint64(len(body)) {
		log.Printf("TopSAP [%d] first IPv4 packet received bytes=%d", s.id, len(body))
	}
	return copy(b, body), nil
}
func (s *Session) Write(b []byte) (int, error) {
	if !validIPv4(b) {
		return 0, errProtocol
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return 0, err
	}
	if err := writeAll(s.conn, frame(15, b)); err != nil {
		log.Printf("TopSAP [%d] tunnel write failed: %v", s.id, err)
		return 0, err
	}
	if s.txBytes.Add(uint64(len(b))) == uint64(len(b)) {
		log.Printf("TopSAP [%d] first IPv4 packet sent bytes=%d protocol=%d source_matches_vip=%t ip_checksum_ok=%t", s.id, len(b), b[9], bytes.Equal(b[12:16], s.ip.AsSlice()), ipv4HeaderChecksumOK(b))
	}
	return len(b), nil
}
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		log.Printf("TopSAP [%d] closing rx_bytes=%d tx_bytes=%d", s.id, s.rxBytes.Load(), s.txBytes.Load())
		s.cancel()
		if s.stopWatch != nil {
			s.stopWatch()
		}
		_ = s.conn.Close()
		s.logout()
		s.transport.CloseIdleConnections()
		log.Printf("TopSAP [%d] closed", s.id)
	})
	return nil
}

func (s *Session) logout() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.post(ctx, "logout", nil, nil); err != nil {
		log.Printf("TopSAP [%d] logout failed: %v", s.id, err)
	} else {
		log.Printf("TopSAP [%d] logout completed", s.id)
	}
}
