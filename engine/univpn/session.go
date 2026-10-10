package univpn

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

type Options struct {
	Server, Domain, Username, Password string
	TLSConfig                          *tls.Config
	DialContext                        func(context.Context, string, string) (net.Conn, error)
}

type channel struct {
	conn *tls.Conn
	mu   sync.Mutex
}

func (c *channel) write(packet []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	n, err := c.conn.Write(packet)
	if err == nil && n != len(packet) {
		return io.ErrShortWrite
	}
	return err
}

func (c *channel) send(command byte, body []byte) error {
	packet, err := frame(command, body)
	if err != nil {
		return err
	}
	return c.write(packet)
}

func (c *channel) exchange(command byte, body []byte) ([]byte, error) {
	if err := c.send(command, body); err != nil {
		return nil, err
	}
	got, reply, err := readFrame(c.conn)
	if err != nil {
		return nil, err
	}
	if got != command {
		return nil, errProtocol
	}
	return reply, nil
}

// Session carries complete IPv4 packets; the caller supplies the IP/TCP/UDP stack.
type Session struct {
	control, data *channel
	ip            netip.Addr
	routes        []netip.Prefix
	token         [32]byte
	done          chan struct{}
	closeOnce     sync.Once
	closeErr      error
	mu            sync.Mutex
	cause         error
}

// Dial authenticates once. Reauthentication belongs to the application, not this transport.
func Dial(ctx context.Context, opts Options) (*Session, error) {
	host, _, err := net.SplitHostPort(opts.Server)
	if err != nil || host == "" || opts.Username == "" || opts.Password == "" {
		return nil, errors.New("UniVPN 服务器、账号或密码配置不完整")
	}
	domain := opts.Domain
	if domain == "" {
		domain = host
	}
	if len(domain) > 255 || strings.ContainsRune(domain+opts.Username+opts.Password, 0) {
		return nil, errors.New("UniVPN 认证字段无效")
	}
	cfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if opts.TLSConfig != nil {
		cfg = opts.TLSConfig.Clone()
		if cfg.ServerName == "" {
			cfg.ServerName = host
		}
	}
	dial := opts.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	setupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	connect := func() (*channel, func() bool, error) {
		raw, err := dial(setupCtx, "tcp", opts.Server)
		if err != nil {
			return nil, nil, err
		}
		stop := context.AfterFunc(setupCtx, func() { _ = raw.Close() })
		conn := tls.Client(raw, cfg)
		deadline, _ := setupCtx.Deadline()
		if err = conn.SetDeadline(deadline); err == nil {
			err = conn.HandshakeContext(setupCtx)
		}
		if err != nil {
			stop()
			_ = conn.Close()
			return nil, nil, err
		}
		return &channel{conn: conn}, stop, nil
	}
	control, stopControl, err := connect()
	if err != nil {
		return nil, err
	}
	defer stopControl()
	ready := false
	defer func() {
		if !ready {
			_ = control.conn.Close()
		}
	}()

	first := make([]byte, 340)
	copy(first, []byte{0xfe, 0xfc, 0xef, 0xbe, 0xc1, 0x92, 0xa4, 0xd6})
	binary.BigEndian.PutUint16(first[12:], 29)
	binary.BigEndian.PutUint16(first[14:], 324)
	copy(first[80:336], domain)
	first[337] = 1
	if err := control.write(first); err != nil {
		return nil, err
	}
	command, reply, err := readFrame(control.conn)
	if err != nil {
		return nil, err
	}
	if command != 0 || len(reply) != 4 || binary.BigEndian.Uint32(reply) != 0 {
		return nil, errProtocol
	}
	auth := make([]byte, 260)
	copy(auth, domain)
	mac := stableMAC(opts.Username, host)
	for i, value := range [][]byte{authValue(opts.Username), authValue(opts.Password), []byte(mac)} {
		auth, err = appendTLV(auth, uint16(i+1), value)
		if err != nil {
			return nil, err
		}
	}
	reply, err = control.exchange(1, auth)
	clear(auth)
	if err != nil {
		return nil, err
	}
	token, err := parseToken(reply)
	if err != nil {
		return nil, err
	}
	reply, err = control.exchange(3, token[:])
	if err != nil {
		return nil, err
	}
	ip, routes, err := parseVIP(reply)
	if err != nil {
		return nil, err
	}

	data, stopData, err := connect()
	if err != nil {
		return nil, err
	}
	defer stopData()
	defer func() {
		if !ready {
			_ = data.conn.Close()
		}
	}()
	controlKey := control.conn.ConnectionState().PeerCertificates[0].RawSubjectPublicKeyInfo
	dataKey := data.conn.ConnectionState().PeerCertificates[0].RawSubjectPublicKeyInfo
	if !bytes.Equal(controlKey, dataKey) {
		return nil, errors.New("UniVPN 数据通道证书与控制通道不一致")
	}
	reply, err = data.exchange(0x64, token[:])
	if err != nil {
		return nil, err
	}
	if len(reply) != 1 || reply[0] != 0 {
		return nil, errProtocol
	}
	if err := control.conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	if err := data.conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	if err := setupCtx.Err(); err != nil {
		return nil, err
	}
	stopControl()
	stopData()
	s := &Session{control: control, data: data, ip: ip, routes: routes, token: token, done: make(chan struct{})}
	ready = true
	go s.readControl()
	go s.heartbeats()
	go func() {
		select {
		case <-ctx.Done():
			s.fail(ctx.Err())
		case <-s.done:
		}
	}()
	return s, nil
}

func (s *Session) IP() netip.Addr { return s.ip }
func (s *Session) Routes() []netip.Prefix {
	return append([]netip.Prefix(nil), s.routes...)
}

func (s *Session) fail(err error) {
	s.mu.Lock()
	if s.cause == nil {
		s.cause = err
	}
	s.mu.Unlock()
	_ = s.Close()
}

func (s *Session) failure(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cause != nil {
		return s.cause
	}
	return err
}

func (s *Session) readControl() {
	for {
		command, _, err := readFrame(s.control.conn)
		if err != nil {
			s.fail(err)
			return
		}
		if command != 6 {
			s.fail(errProtocol)
			return
		}
	}
}

func (s *Session) heartbeats() {
	controlTick := time.NewTicker(10 * time.Second)
	dataTick := time.NewTicker(15 * time.Second)
	defer controlTick.Stop()
	defer dataTick.Stop()
	start := time.Now()
	for {
		var err error
		select {
		case <-s.done:
			return
		case <-controlTick.C:
			body := append([]byte(nil), s.token[:]...)
			body = binary.BigEndian.AppendUint32(body, uint32(time.Since(start).Milliseconds()))
			err = s.control.send(6, body)
		case <-dataTick.C:
			err = s.data.send(0x65, nil)
		}
		if err != nil {
			s.fail(err)
			return
		}
	}
}

func (s *Session) Read(buffer []byte) (int, error) {
	for {
		command, body, err := readFrame(s.data.conn)
		if err != nil {
			s.fail(err)
			return 0, s.failure(err)
		}
		if command == 0x65 {
			continue
		}
		if command != 0x66 || len(body) < 28 || !bytes.Equal(body[:8], make([]byte, 8)) || !validIPv4(body[8:]) {
			s.fail(errProtocol)
			return 0, errProtocol
		}
		if len(body)-8 > len(buffer) {
			return 0, io.ErrShortBuffer
		}
		return copy(buffer, body[8:]), nil
	}
}

func (s *Session) Write(packet []byte) (int, error) {
	if len(packet) > 65527 || !validIPv4(packet) {
		return 0, errProtocol
	}
	body := append(make([]byte, 8, len(packet)+8), packet...)
	if err := s.data.send(0x66, body); err != nil {
		s.fail(err)
		return 0, s.failure(err)
	}
	return len(packet), nil
}

func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		close(s.done)
		s.closeErr = errors.Join(s.control.conn.Close(), s.data.conn.Close())
	})
	return s.closeErr
}
