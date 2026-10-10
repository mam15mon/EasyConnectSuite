package engine

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/stack/gvisor"
)

// This gateway uses literal wire values and a small IPv4 echo endpoint. It does
// not reuse the production UniVPN encoder or authentication parser.
func gatewayFrame(conn net.Conn, cmd byte, body []byte) error {
	h := []byte{0xc1, 0x92, 0xa4, 0xd6, 1, cmd, 0, 0}
	binary.BigEndian.PutUint16(h[6:], uint16(len(body)))
	_, err := conn.Write(append(h, body...))
	return err
}
func gatewayRead(conn net.Conn) (byte, []byte, error) {
	h := make([]byte, 8)
	if _, err := io.ReadFull(conn, h); err != nil {
		return 0, nil, err
	}
	if !bytes.Equal(h[:5], []byte{0xc1, 0x92, 0xa4, 0xd6, 1}) {
		return 0, nil, errors.New("wrong wire magic")
	}
	b := make([]byte, binary.BigEndian.Uint16(h[6:]))
	_, err := io.ReadFull(conn, b)
	return h[5], b, err
}
func wireChecksum(b []byte) uint16 {
	var sum uint32
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b))
		b = b[2:]
	}
	if len(b) > 0 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 > 0 {
		sum = (sum & 65535) + (sum >> 16)
	}
	return ^uint16(sum)
}
func echoIPv4(packet []byte) []byte {
	if len(packet) < 28 || packet[0] != 0x45 {
		return nil
	}
	transport := packet[20:]
	var reply []byte
	switch packet[9] {
	case 17:
		reply = append([]byte(nil), packet...)
		copy(reply[20:22], transport[2:4])
		copy(reply[22:24], transport[:2])
		reply[26], reply[27] = 0, 0
	case 6:
		if len(transport) < 20 {
			return nil
		}
		hdr := int(transport[12]>>4) * 4
		if hdr < 20 || hdr > len(transport) {
			return nil
		}
		data := transport[hdr:]
		flags := transport[13]
		seq := binary.BigEndian.Uint32(transport[4:])
		ack := binary.BigEndian.Uint32(transport[8:])
		outFlags := byte(0x18)
		if flags&2 != 0 {
			ack = 1000
			seq++
			outFlags = 0x12
		} else if len(data) == 0 {
			if flags&1 == 0 {
				return nil
			}
			seq++
			outFlags = 0x11
		} else {
			seq += uint32(len(data))
		}
		reply = make([]byte, 40+len(data))
		copy(reply[:20], packet[:20])
		copy(reply[20:22], transport[2:4])
		copy(reply[22:24], transport[:2])
		binary.BigEndian.PutUint32(reply[24:], ack)
		binary.BigEndian.PutUint32(reply[28:], seq)
		reply[32], reply[33] = 0x50, outFlags
		binary.BigEndian.PutUint16(reply[34:], 65535)
		copy(reply[40:], data)
	default:
		return nil
	}
	copy(reply[12:16], packet[16:20])
	copy(reply[16:20], packet[12:16])
	reply[10], reply[11] = 0, 0
	binary.BigEndian.PutUint16(reply[2:], uint16(len(reply)))
	binary.BigEndian.PutUint16(reply[10:], wireChecksum(reply[:20]))
	if packet[9] == 6 {
		pseudo := append([]byte(nil), reply[12:20]...)
		pseudo = append(pseudo, 0, 6)
		pseudo = binary.BigEndian.AppendUint16(pseudo, uint16(len(reply)-20))
		pseudo = append(pseudo, reply[20:]...)
		binary.BigEndian.PutUint16(reply[36:], wireChecksum(pseudo))
	}
	return reply
}
func integrationGateway(t *testing.T, reject bool) (string, *tls.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var conns []net.Conn
	closed := false
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2; i++ {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			if closed {
				mu.Unlock()
				raw.Close()
				return
			}
			conns = append(conns, raw)
			mu.Unlock()
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				conn := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
				defer conn.Close()
				if conn.Handshake() != nil {
					return
				}
				if index == 0 {
					first := make([]byte, 340)
					if _, err := io.ReadFull(conn, first); err != nil {
						return
					}
					if gatewayFrame(conn, 0, make([]byte, 4)) != nil {
						return
					}
					cmd, _, err := gatewayRead(conn)
					if err != nil || cmd != 1 {
						return
					}
					body := make([]byte, 44)
					body[9], body[11] = 3, 32
					copy(body[12:], bytes.Repeat([]byte{0x5a}, 32))
					if reject {
						body[3] = 1
					}
					if gatewayFrame(conn, 1, body) != nil || reject {
						return
					}
					cmd, _, err = gatewayRead(conn)
					if err != nil || cmd != 3 {
						return
					}
					vip := []byte{0, 0, 0, 0, 0, 1, 0, 5, 10, 90, 0, 2, 32, 0, 6, 0, 5, 198, 51, 100, 0, 24}
					if gatewayFrame(conn, 3, vip) != nil {
						return
					}
					for {
						cmd, _, err = gatewayRead(conn)
						if err != nil {
							return
						}
						if gatewayFrame(conn, cmd, nil) != nil {
							return
						}
					}
				} else {
					cmd, _, err := gatewayRead(conn)
					if err != nil || cmd != 0x64 {
						return
					}
					if gatewayFrame(conn, 0x64, []byte{0}) != nil {
						return
					}
					for {
						cmd, body, err := gatewayRead(conn)
						if err != nil {
							return
						}
						if cmd == 0x65 {
							if gatewayFrame(conn, cmd, nil) != nil {
								return
							}
							continue
						}
						if cmd != 0x66 || len(body) < 28 {
							return
						}
						reply := echoIPv4(body[8:])
						if reply != nil {
							if gatewayFrame(conn, 0x66, append(make([]byte, 8), reply...)) != nil {
								return
							}
						}
					}
				}
			}(i)
		}
	}()
	t.Cleanup(func() {
		mu.Lock()
		closed = true
		listener.Close()
		for _, c := range conns {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return listener.Addr().String(), &tls.Config{RootCAs: roots}
}
func integrationClient(t *testing.T, reject bool) *uniVPNClient {
	server, cfg := integrationGateway(t, reject)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c := &uniVPNClient{ctx: ctx, cancel: cancel}
	c.options.Server = server
	c.options.Username = "test"
	c.options.Password = "test"
	c.options.TLSConfig = cfg
	t.Cleanup(c.Close)
	return c
}
func TestUniVPNAdapterGVisorTCPAndUDP(t *testing.T) {
	c := integrationClient(t, false)
	if err := c.Setup(); err != nil {
		t.Fatal(err)
	}
	stack, err := gvisor.NewOwnedStack(c, func() bool { return c.ctx.Err() != nil })
	if err != nil {
		t.Fatal(err)
	}
	defer stack.Close()
	go stack.Run()
	conf := configs.Default()
	conf.DisableZJUConfig = true
	conf.DisableRemoteDNS = true
	policy, err := loadPolicy(conf, c, stack)
	if err != nil {
		t.Fatal(err)
	}
	defer policy.resolver.Close()
	if len(policy.ips) != 1 || !policy.ips[0].IPMin.Equal(net.ParseIP("198.51.100.0")) || !policy.ips[0].IPMax.Equal(net.ParseIP("198.51.100.255")) || policy.useRemoteDNS || policy.dnsName != "" {
		t.Fatal("incorrect UniVPN policy")
	}
	for _, network := range []string{"tcp", "udp"} {
		t.Run(network, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, err := policy.dialer.DialIPPort(ctx, network, "198.51.100.9:1234")
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			payload := []byte("UniVPN native Go echo")
			if _, err = conn.Write(payload); err != nil {
				t.Fatal(err)
			}
			reply := make([]byte, len(payload))
			if network == "tcp" {
				_, err = io.ReadFull(conn, reply)
			} else {
				_, err = conn.Read(reply)
			}
			if err != nil || !bytes.Equal(reply, payload) {
				t.Fatalf("gVisor %s round trip: %q %v", network, reply, err)
			}
		})
	}
}
func TestUniVPNAdapterAuthenticationRefusalIsTerminal(t *testing.T) {
	c := integrationClient(t, true)
	if err := c.Setup(); !errors.Is(err, easyconnect.ErrLoginRejected) {
		t.Fatalf("lost terminal authentication error: %v", err)
	}
}

type certificateTestUnderlay struct{ net.Dialer }

func (*certificateTestUnderlay) ExcludeIP(net.IP) {}

func TestUniVPNAdapterExplicitCertificateSkip(t *testing.T) {
	for _, skip := range []bool{false, true} {
		t.Run(map[bool]string{false: "verify by default", true: "explicit skip"}[skip], func(t *testing.T) {
			server, _ := integrationGateway(t, false)
			host, portText, err := net.SplitHostPort(server)
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				t.Fatal(err)
			}
			conf := configs.Default()
			conf.ServerAddress, conf.ServerPort = host, port
			conf.Username, conf.Password = "test", "test"
			c := newUniVPNClient(context.Background(), conf, "", skip, &certificateTestUnderlay{})
			defer c.Close()
			err = c.Setup()
			if skip {
				if err != nil {
					t.Fatal(err)
				}
				if ip, err := c.IP(); err != nil || !ip.Equal(net.ParseIP("10.90.0.2")) {
					t.Fatal("authentication did not complete", ip, err)
				}
			} else {
				var authorityErr x509.UnknownAuthorityError
				if !errors.As(err, &authorityErr) {
					t.Fatal("untrusted certificate was not rejected", err)
				}
			}
		})
	}
}

func TestTwoUniVPNTunnelsDoNotShareNetworkOrShutdown(t *testing.T) {
	first, second := integrationClient(t, false), integrationClient(t, false)
	if err := first.Setup(); err != nil {
		t.Fatal(err)
	}
	if err := second.Setup(); err != nil {
		t.Fatal(err)
	}
	a, err := gvisor.NewOwnedStack(first, func() bool { return first.ctx.Err() != nil })
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	go a.Run()
	b, err := gvisor.NewOwnedStack(second, func() bool { return second.ctx.Err() != nil })
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	go b.Run()
	first.Close()
	a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := b.DialUDP(ctx, &net.UDPAddr{IP: net.ParseIP("198.51.100.9"), Port: 1234})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	payload := []byte("second VPN survives first shutdown")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, len(payload))
	if _, err := conn.Read(reply); err != nil || !bytes.Equal(reply, payload) {
		t.Fatal("second network stopped", err)
	}
}
