package univpn

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testCertificate(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, cert
}

type mockGateway struct {
	listener      net.Listener
	tlsConfigs    [2]*tls.Config
	roots         *x509.CertPool
	reject, stall bool
	malformed     bool
	mu            sync.Mutex
	conns         []net.Conn
	closed        bool
	wg            sync.WaitGroup
	accepted      atomic.Int32
	beats         chan byte
	problems      chan error
}

func newGateway(t *testing.T, reject, mismatch bool) *mockGateway {
	t.Helper()
	first, cert := testCertificate(t)
	second := first
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	if mismatch {
		var other *x509.Certificate
		second, other = testCertificate(t)
		roots.AddCert(other)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := &mockGateway{
		listener: listener, roots: roots, reject: reject,
		tlsConfigs: [2]*tls.Config{{Certificates: []tls.Certificate{first}}, {Certificates: []tls.Certificate{second}}},
		beats:      make(chan byte, 8), problems: make(chan error, 8),
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		for i := 0; i < 2; i++ {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			g.mu.Lock()
			if g.closed {
				g.mu.Unlock()
				_ = raw.Close()
				return
			}
			g.conns = append(g.conns, raw)
			g.mu.Unlock()
			g.accepted.Add(1)
			g.wg.Add(1)
			go func(index int) {
				defer g.wg.Done()
				conn := tls.Server(raw, g.tlsConfigs[index])
				defer conn.Close()
				if err := conn.Handshake(); err != nil {
					return
				}
				if index == 0 {
					g.control(conn)
				} else {
					g.data(conn)
				}
			}(i)
		}
	}()
	t.Cleanup(func() {
		g.mu.Lock()
		g.closed = true
		_ = g.listener.Close()
		for _, conn := range g.conns {
			_ = conn.Close()
		}
		g.mu.Unlock()
		g.wg.Wait()
		select {
		case err := <-g.problems:
			t.Error(err)
		default:
		}
	})
	return g
}

func (g *mockGateway) options() Options {
	return Options{Server: g.listener.Addr().String(), Domain: "localhost",
		Username: "mockuser", Password: "p&+%", TLSConfig: &tls.Config{RootCAs: g.roots}}
}

func (g *mockGateway) send(conn net.Conn, command byte, body []byte) error {
	packet, err := frame(command, body)
	if err != nil {
		return err
	}
	for _, part := range [][]byte{packet[:3], packet[3:8], packet[8:]} {
		if len(part) != 0 {
			if _, err := conn.Write(part); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *mockGateway) control(conn net.Conn) {
	first := make([]byte, 340)
	if _, err := io.ReadFull(conn, first); err != nil {
		return
	}
	if !bytes.Equal(first[:8], []byte{0xfe, 0xfc, 0xef, 0xbe, 0xc1, 0x92, 0xa4, 0xd6}) ||
		binary.BigEndian.Uint16(first[14:16]) != 324 || string(first[80:89]) != "localhost" {
		g.problems <- errors.New("wrong FirstConnect")
		return
	}
	if g.stall {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	if err := g.send(conn, 0, make([]byte, 4)); err != nil {
		return
	}
	command, auth, err := readFrame(conn)
	if err != nil {
		return
	}
	if command != 1 || len(auth) < 260 || string(auth[:9]) != "localhost" {
		g.problems <- errors.New("wrong authentication frame")
		return
	}
	fields := 0
	err = parseTLVs(auth[260:], func(kind uint16, value []byte) error {
		fields++
		if kind == 1 && string(value) != "mockuser\x00" || kind == 2 && string(value) != "p%26%2B%25\x00" ||
			kind == 3 && (len(value) != 17 || string(value[:3]) != "02:") {
			return errors.New("wrong authentication field")
		}
		return nil
	})
	if err != nil || fields != 3 {
		g.problems <- errors.New("invalid authentication TLVs")
		return
	}
	token := bytes.Repeat([]byte{0x5a}, 32)
	reply, _ := appendTLV(make([]byte, 8), 3, token)
	if g.reject {
		reply[3] = 1
	}
	if err := g.send(conn, 1, reply); err != nil || g.reject {
		return
	}
	command, body, err := readFrame(conn)
	if err != nil {
		return
	}
	if command != 3 || !bytes.Equal(body, token) {
		g.problems <- errors.New("wrong REQVIP")
		return
	}
	reply, _ = appendTLV(make([]byte, 4), 1, []byte{10, 90, 0, 2, 32})
	reply, _ = appendTLV(reply, 6, []byte{198, 51, 100, 0, 24})
	if err := g.send(conn, 3, reply); err != nil {
		return
	}
	for {
		command, body, err = readFrame(conn)
		if err != nil {
			return
		}
		if command != 6 || len(body) != 36 || !bytes.Equal(body[:32], token) {
			g.problems <- errors.New("wrong control heartbeat")
			return
		}
		g.beats <- command
		if err := g.send(conn, 6, nil); err != nil {
			return
		}
	}
}

func (g *mockGateway) data(conn net.Conn) {
	command, token, err := readFrame(conn)
	if err != nil {
		return
	}
	if command != 0x64 || !bytes.Equal(token, bytes.Repeat([]byte{0x5a}, 32)) {
		g.problems <- errors.New("wrong DATA_CONNECT")
		return
	}
	if err := g.send(conn, 0x64, []byte{0}); err != nil {
		return
	}
	for {
		command, body, err := readFrame(conn)
		if err != nil {
			return
		}
		if command == 0x65 {
			g.beats <- command
		}
		if g.malformed && command == 0x66 {
			body[8] = 0x65 // IPv6 in a negotiated IPv4-only session.
		}
		if err := g.send(conn, command, body); err != nil {
			return
		}
	}
}

func TestSessionPacketsAndHeartbeats(t *testing.T) {
	g := newGateway(t, false, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := Dial(ctx, g.options())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.IP() != netip.MustParseAddr("10.90.0.2") || len(s.Routes()) != 1 ||
		s.Routes()[0] != netip.MustParsePrefix("198.51.100.0/24") {
		t.Fatal("wrong assigned IP/routes")
	}
	for _, protocol := range []byte{6, 17, 1} {
		packet := make([]byte, 36)
		packet[0], packet[9] = 0x45, protocol
		binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
		if n, err := s.Write(packet); err != nil || n != len(packet) {
			t.Fatalf("write: %d, %v", n, err)
		}
		buffer := make([]byte, 1500)
		n, err := s.Read(buffer)
		if err != nil || !bytes.Equal(buffer[:n], packet) {
			t.Fatalf("packet round trip: %v", err)
		}
	}
	deadline := time.After(18 * time.Second)
	seen := map[byte]bool{}
	for len(seen) != 2 {
		select {
		case beat := <-g.beats:
			seen[beat] = true
		case <-deadline:
			t.Fatal("missing control or data heartbeat")
		}
	}
}

func TestRejectedPasswordStopsBeforeDataConnection(t *testing.T) {
	g := newGateway(t, true, false)
	s, err := Dial(context.Background(), g.options())
	if s != nil || !errors.Is(err, ErrLoginRejected) || g.accepted.Load() != 1 {
		t.Fatalf("session=%v error=%v connections=%d", s, err, g.accepted.Load())
	}
}

func TestDataChannelIdentityMustMatchControl(t *testing.T) {
	g := newGateway(t, false, true)
	s, err := Dial(context.Background(), g.options())
	if s != nil || err == nil {
		t.Fatal("trusted but different data-channel key accepted")
	}
}

func TestUntrustedCertificateRejected(t *testing.T) {
	g := newGateway(t, false, false)
	opts := g.options()
	opts.TLSConfig = nil
	if s, err := Dial(context.Background(), opts); s != nil || err == nil {
		t.Fatal("untrusted gateway accepted")
	}
}

func TestControlLossAndCancellationUnblockPacketRead(t *testing.T) {
	for _, drop := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "control EOF"}[drop], func(t *testing.T) {
			g := newGateway(t, false, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, err := Dial(ctx, g.options())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			result := make(chan error, 1)
			go func() { _, err := s.Read(make([]byte, 1500)); result <- err }()
			if drop {
				g.mu.Lock()
				_ = g.conns[0].Close()
				g.mu.Unlock()
			} else {
				cancel()
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("blocked read returned success")
				}
				if !drop && !errors.Is(err, context.Canceled) {
					t.Fatalf("lost cancellation cause: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("packet read remained blocked")
			}
		})
	}
}

func TestCancelDuringAuthentication(t *testing.T) {
	g := newGateway(t, false, false)
	g.stall = true
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if s, err := Dial(ctx, g.options()); s != nil || err == nil {
		t.Fatal("stalled authentication was not canceled")
	}
}

func TestMalformedPacketClosesSession(t *testing.T) {
	g := newGateway(t, false, false)
	g.malformed = true
	s, err := Dial(context.Background(), g.options())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	packet := make([]byte, 20)
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], 20)
	if _, err := s.Write(packet); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(make([]byte, 1500)); !errors.Is(err, errProtocol) {
		t.Fatalf("malformed packet accepted: %v", err)
	}
	select {
	case <-s.done:
	default:
		t.Fatal("malformed session remained open")
	}
}

func TestMalformedFramesAndResourceTLVs(t *testing.T) {
	encoded, _ := frame(1, []byte{1, 2, 3})
	for _, size := range []int{1, 7, 9} {
		if _, _, err := readFrame(bytes.NewReader(encoded[:size])); err == nil {
			t.Fatalf("truncated frame length %d accepted", size)
		}
	}
	encoded[0] = 0
	if _, _, err := readFrame(bytes.NewReader(encoded)); !errors.Is(err, errProtocol) {
		t.Fatal("invalid magic accepted")
	}
	for _, body := range [][]byte{{0}, {0, 1, 0, 2, 1}} {
		if err := parseTLVs(body, func(uint16, []byte) error { return nil }); err == nil {
			t.Fatal("truncated TLV accepted")
		}
	}
	for _, route := range [][]byte{{198, 51, 100, 1, 24}, {198, 51, 100, 0, 33}} {
		vip, _ := appendTLV(make([]byte, 4), 1, []byte{10, 90, 0, 2, 32})
		vip, _ = appendTLV(vip, 6, route)
		if _, _, err := parseVIP(vip); err == nil {
			t.Fatal("invalid route accepted")
		}
	}
}
