package topsec

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type gateway struct {
	server                              *httptest.Server
	roots                               *x509.CertPool
	reject, noResources, stall, corrupt bool
	logins, logouts                     atomic.Int32
	mu                                  sync.Mutex
	conns                               []net.Conn
	wg                                  sync.WaitGroup
}

func newGateway(t *testing.T) *gateway {
	t.Helper()
	g := &gateway{}
	g.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			g.mu.Lock()
			g.conns = append(g.conns, conn)
			g.wg.Add(1)
			g.mu.Unlock()
			defer g.wg.Done()
			defer conn.Close()
			g.tunnel(t, conn, rw.Reader)
			return
		}
		if r.Method != "POST" {
			t.Error("wrong HTTP method")
			http.Error(w, "method", 405)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		switch r.URL.Path {
		case "/vone/login/auth_param":
			if g.stall {
				<-r.Context().Done()
				return
			}
			fmt.Fprint(w, `{pswd:"on",check_client_before_login:"off",auth_protocol:"0",data_protocol:"0",twoway_auth_switch:"off"}`)
		case "/vone/login/user_pswd":
			g.logins.Add(1)
			if r.Form.Get("U") != "bW9ja3VzZXI=" || r.Form.Get("P") != "IHAmKyUg" || r.Form.Get("S_CODE") != "tOn2cYFE+wqTmNHkz2AA7g==" || !r.Form.Has("HASH") || r.Form.Get("HASH") != "" || len(r.Form) != 4 {
				t.Error("authentication differs from independent vectors")
			}
			if g.reject {
				fmt.Fprint(w, `{"Ret":-1001,"ErrMsg":"refused"}`)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "topafasfasfassession_id", Value: "synthetic-session", Path: "/", Secure: true, HttpOnly: true})
			fmt.Fprint(w, `{"Ret":0,"user_param":{"topafasfasfassession_id":"synthetic-session","netacc":"on","alg":"standard","protocal":"standard","compress":"off"},"net_param":{"vip_v4":"10.91.0.2","dns1_v4":"0.0.0.0","dns2_v4":"0.0.0.0"}}`)
		case "/vone/login/get_config_info":
			cookie, err := r.Cookie("topafasfasfassession_id")
			if err != nil || cookie.Value != "synthetic-session" || !r.Form.Has("HASH") || r.Form.Get("HASH") != "" || r.Form.Get("TYPE") != "phone" {
				t.Error("configuration request lost identity/cache semantics")
			}
			if g.noResources {
				fmt.Fprint(w, `{"Ret":0}`)
				return
			}
			fmt.Fprint(w, `{"Ret":0,"res_list":[{"module":"na","protocol":"ip","ip":"198.51.100.0","mask":"255.255.255.0","port":"","domain":""}]}`)
		case "/vone/login/logout":
			g.logouts.Add(1)
			fmt.Fprint(w, `{"Ret":0}`)
		default:
			t.Error("unexpected authentication endpoint")
			http.Error(w, "path", 404)
		}
	}))
	g.roots = x509.NewCertPool()
	g.roots.AddCert(g.server.Certificate())
	t.Cleanup(func() {
		g.mu.Lock()
		for _, conn := range g.conns {
			_ = conn.Close()
		}
		g.mu.Unlock()
		g.server.Close()
		g.wg.Wait()
	})
	return g
}
func (g *gateway) options() Options {
	return Options{Server: strings.TrimPrefix(g.server.URL, "https://"), Username: "mockuser", Password: " p&+% ", TLSConfig: &tls.Config{RootCAs: g.roots}}
}
func readClient(r io.Reader) (uint32, []byte, error) {
	h := make([]byte, 16)
	if _, err := io.ReadFull(r, h); err != nil {
		return 0, nil, err
	}
	if binary.BigEndian.Uint32(h) != 0 || binary.BigEndian.Uint32(h[12:]) != 1 || binary.BigEndian.Uint32(h[8:]) > 65535 {
		return 0, nil, errors.New("wrong client header")
	}
	body := make([]byte, binary.BigEndian.Uint32(h[8:]))
	_, err := io.ReadFull(r, body)
	return binary.BigEndian.Uint32(h[4:]), body, err
}
func (g *gateway) tunnel(t *testing.T, conn net.Conn, r *bufio.Reader) {
	command, body, err := readClient(r)
	if err != nil {
		return
	}
	want := make([]byte, 328)
	want[3] = 4
	copy(want[72:], "synthetic-session")
	if command != 1 || !bytes.Equal(body, want) {
		t.Error("incorrect setup payload")
		return
	}
	ack := []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 1}
	// Deliberately fragment headers independently of TLS/socket read boundaries.
	if _, err = conn.Write(ack[:3]); err != nil {
		return
	}
	if _, err = conn.Write(ack[3:]); err != nil {
		return
	}
	command, body, err = readClient(r)
	if err != nil {
		return
	}
	if command != 7 || !bytes.Equal(body, make([]byte, 8)) {
		t.Error("incorrect one-time command 7")
		return
	}
	for {
		command, body, err = readClient(r)
		if err != nil {
			return
		}
		if command != 15 {
			t.Error("unexpected data command")
			return
		}
		head := []byte{0, 0, 0xaa, 0xbb, 0, 0, 0, 15, 0, 0, 0, 0, 0xee, 0xff, 8, 0}
		binary.BigEndian.PutUint32(head[8:], uint32(len(body)))
		if g.corrupt {
			head[0] = 1
		}
		if _, err = conn.Write(append(head, body...)); err != nil {
			return
		}
	}
}
func TestSessionIPv4AndIsolation(t *testing.T) {
	g := newGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, err := dial(ctx, g.options(), "tOn2cYFE+wqTmNHkz2AA7g==")
	if err != nil {
		t.Fatal(err)
	}
	b, err := dial(context.Background(), g.options(), "tOn2cYFE+wqTmNHkz2AA7g==")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if a.IP().String() != "10.91.0.2" || len(a.Routes()) != 1 || a.Routes()[0].String() != "198.51.100.0/24" || len(a.DNSServers()) != 0 {
		t.Fatal("incorrect session configuration")
	}
	for _, protocol := range []byte{6, 17} {
		packet := make([]byte, 40)
		packet[0] = 0x45
		packet[3] = 40
		packet[8] = 64
		packet[9] = protocol
		if _, err = a.Write(packet); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 1500)
		n, err := a.Read(got)
		if err != nil || !bytes.Equal(got[:n], packet) {
			t.Fatal("packet round trip failed", err)
		}
	}
	blocked := make(chan error, 1)
	go func() { _, err := a.Read(make([]byte, 1500)); blocked <- err }()
	cancel()
	select {
	case err := <-blocked:
		if err == nil {
			t.Fatal("cancel did not interrupt read")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel leaked reader")
	}
	_ = a.Close()
	_ = a.Close()
	packet := make([]byte, 28)
	packet[0] = 0x45
	packet[3] = 28
	packet[9] = 17
	if _, err = b.Write(packet); err != nil {
		t.Fatal("stopping A interrupted B", err)
	}
	if _, err = b.Read(make([]byte, 1500)); err != nil {
		t.Fatal("B stopped", err)
	}
	if g.logouts.Load() != 1 {
		t.Fatal("logout was not idempotent")
	}
}
func TestRefusalAndMissingResources(t *testing.T) {
	for _, mode := range []string{"reject", "resources", "certificate"} {
		t.Run(mode, func(t *testing.T) {
			g := newGateway(t)
			g.reject = mode == "reject"
			g.noResources = mode == "resources"
			opts := g.options()
			if mode == "certificate" {
				opts.TLSConfig = nil
			}
			s, err := dial(context.Background(), opts, "tOn2cYFE+wqTmNHkz2AA7g==")
			if err == nil {
				_ = s.Close()
				t.Fatal("invalid session accepted")
			}
			if mode == "reject" {
				var refusal *AuthenticationError
				if !errors.Is(err, ErrLoginRejected) || !errors.As(err, &refusal) || refusal.Code != -1001 || g.logins.Load() != 1 {
					t.Fatal("refusal lost or retried", err)
				}
			}
			if mode == "certificate" && g.logins.Load() != 0 {
				t.Fatal("credentials sent before certificate verification")
			}
			if mode == "resources" && g.logouts.Load() != 1 {
				t.Fatal("authenticated startup failure leaked session")
			}
		})
	}
}
func TestSetupCancellationAndCorruptStream(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		g := newGateway(t)
		g.stall = true
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := dial(ctx, g.options(), "tOn2cYFE+wqTmNHkz2AA7g==")
		if err == nil {
			t.Fatal("cancel accepted")
		}
		if g.logins.Load() != 0 {
			t.Fatal("login after cancel")
		}
	})
	t.Run("corrupt", func(t *testing.T) {
		g := newGateway(t)
		g.corrupt = true
		s, err := dial(context.Background(), g.options(), "tOn2cYFE+wqTmNHkz2AA7g==")
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		b := make([]byte, 28)
		b[0] = 0x45
		b[3] = 28
		b[9] = 17
		if _, err = s.Write(b); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Read(make([]byte, 1500)); !errors.Is(err, errProtocol) {
			t.Fatal("corrupt frame accepted", err)
		}
	})
}
