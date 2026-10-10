package engine

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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"EasyConnectSuite/engine/topsec"
	"github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/configs"
	"github.com/mythologyli/zju-connect/stack/gvisor"
)

// The mock uses independent headers and the existing IPv4 echo endpoint. It
// exercises real gVisor TCP/UDP without installing a Windows TUN or routes.
func topSAPGateway(t *testing.T, reject bool) (*topSAPClient, *atomic.Int32) {
	t.Helper()
	var mu sync.Mutex
	var conns []net.Conn
	logins := &atomic.Int32{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
			defer conn.Close()
			cmd, body, err := topSAPReadClient(rw.Reader)
			if err != nil {
				return
			}
			if cmd != 1 || len(body) != 328 || body[3] != 4 || string(bytes.TrimRight(body[72:328], "\x00")) != "mock-session" {
				t.Error("incorrect setup")
				return
			}
			if _, err = conn.Write([]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 1}); err != nil {
				return
			}
			cmd, body, err = topSAPReadClient(rw.Reader)
			if err != nil {
				return
			}
			if cmd != 7 || !bytes.Equal(body, make([]byte, 8)) {
				t.Error("incorrect initial control command")
				return
			}
			for {
				cmd, body, err = topSAPReadClient(rw.Reader)
				if err != nil {
					return
				}
				if cmd != 15 {
					t.Error("incorrect data command")
					return
				}
				packet := echoIPv4(body)
				if packet == nil {
					continue
				}
				head := []byte{0, 0, 0xaa, 0xbb, 0, 0, 0, 15, 0, 0, 0, 0, 0xee, 0xff, 8, 0}
				binary.BigEndian.PutUint32(head[8:], uint32(len(packet)))
				if _, err = conn.Write(append(head, packet...)); err != nil {
					return
				}
			}
		}
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/vone/login/auth_param":
			fmt.Fprint(w, `{pswd:"on",check_client_before_login:"off",auth_protocol:"0",data_protocol:"0",twoway_auth_switch:"off"}`)
		case "/vone/login/user_pswd":
			logins.Add(1)
			if reject {
				fmt.Fprint(w, `{"Ret":-1001}`)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "topafasfasfassession_id", Value: "mock-session", Path: "/", Secure: true})
			fmt.Fprint(w, `{"Ret":0,"user_param":{"topafasfasfassession_id":"mock-session","netacc":"on","alg":"standard","protocal":"standard","compress":"off"},"net_param":{"vip_v4":"10.91.0.2","dns1_v4":"0.0.0.0","dns2_v4":"0.0.0.0"}}`)
		case "/vone/login/get_config_info":
			if err := r.ParseForm(); err != nil || r.Form.Get("TYPE") != "all" || !r.Form.Has("HASH") || r.Form.Get("HASH") != "" {
				t.Error("full resource request differs from native TYPE=all contract")
			}
			fmt.Fprint(w, `{"Ret":0,"res_list":[{"module":"na","protocol":"ip","ip":"198.51.100.0","mask":"255.255.255.0","port":"","domain":""}]}`)
		case "/vone/login/logout":
			fmt.Fprint(w, `{"Ret":0}`)
		default:
			t.Error("unexpected endpoint")
			http.Error(w, "path", 404)
		}
	}))
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	ctx, cancel := context.WithCancel(context.Background())
	c := &topSAPClient{ctx: ctx, cancel: cancel, options: topsec.Options{Server: strings.TrimPrefix(server.URL, "https://"), Username: "test", Password: "test", TLSConfig: &tls.Config{RootCAs: roots}}}
	t.Cleanup(func() {
		c.Close()
		mu.Lock()
		for _, conn := range conns {
			_ = conn.Close()
		}
		mu.Unlock()
		server.Close()
	})
	return c, logins
}
func topSAPReadClient(r *bufio.Reader) (uint32, []byte, error) {
	h := make([]byte, 16)
	if _, err := io.ReadFull(r, h); err != nil {
		return 0, nil, err
	}
	if binary.BigEndian.Uint32(h) != 0 || binary.BigEndian.Uint32(h[12:]) != 1 || binary.BigEndian.Uint32(h[8:]) > 65535 {
		return 0, nil, errors.New("invalid header")
	}
	body := make([]byte, binary.BigEndian.Uint32(h[8:]))
	_, err := io.ReadFull(r, body)
	return binary.BigEndian.Uint32(h[4:]), body, err
}
func TestTopSAPAdapterGVisorTCPAndUDP(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "engine.log")
	capture := &Engine{opts: Options{LogFile: logPath}}
	capture.startLogCapture()
	defer capture.stopLogCapture()
	c, _ := topSAPGateway(t, false)
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
	conf.Protocol = "topsec"
	conf.DisableZJUConfig = true
	policy, err := loadPolicy(conf, c, stack)
	if err != nil {
		t.Fatal(err)
	}
	defer policy.resolver.Close()
	if len(policy.ips) != 1 || policy.useRemoteDNS || policy.dnsName != "" {
		t.Fatal("incorrect resource/DNS policy")
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
			_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
			payload := []byte("TopSAP Go TCP/UDP echo")
			if _, err = conn.Write(payload); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, len(payload))
			if network == "tcp" {
				_, err = io.ReadFull(conn, got)
			} else {
				_, err = conn.Read(got)
			}
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatal("gVisor round trip failed", err)
			}
		})
	}
	c.Close()
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"certificate_verify=true", "endpoint=auth_param status=200", "authentication accepted Ret=0", "TYPE=all", "resources_present=true resources=1", "resources ready routes=1 dns_servers=0", "等待隧道确认 ACK", "tunnel ready", "first IPv4 packet sent", "first IPv4 packet received", "rx_bytes=", "logout completed", "closed"} {
		if !strings.Contains(string(logged), marker) {
			t.Fatalf("missing persistent log marker %q", marker)
		}
	}
	for _, secret := range []string{"mock-session", "dGVzdA==", "S_CODE=", "P=", "U="} {
		if strings.Contains(string(logged), secret) {
			t.Fatal("setup or data logs exposed private authentication fields")
		}
	}
}
func TestTopSAPAdapterRefusalIsTerminal(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "engine.log")
	capture := &Engine{opts: Options{LogFile: logPath}}
	capture.startLogCapture()
	defer capture.stopLogCapture()
	c, count := topSAPGateway(t, true)
	err := c.Setup()
	if !errors.Is(err, topsec.ErrLoginRejected) || !errors.Is(err, easyconnect.ErrLoginRejected) || count.Load() != 1 {
		t.Fatal("authentication refusal lost or retried", err)
	}
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil || !strings.Contains(string(logged), "TopSAP setup failed:") || !strings.Contains(string(logged), "账号密码认证") || !strings.Contains(string(logged), "TopSAP authentication rejected: Ret=-1001") {
		t.Fatal("refusal stage and numeric cause were not persisted", readErr)
	}
}

func TestTopSAPMissingResourcesFailureIsPersisted(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "engine.log")
	capture := &Engine{opts: Options{LogFile: logPath}}
	capture.startLogCapture()
	defer capture.stopLogCapture()
	var logouts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/vone/login/auth_param":
			fmt.Fprint(w, `{pswd:"on",check_client_before_login:"off",auth_protocol:"0",data_protocol:"0",twoway_auth_switch:"off"}`)
		case "/vone/login/user_pswd":
			fmt.Fprint(w, `{"Ret":0,"user_param":{"topafasfasfassession_id":"private-test-token","netacc":"on","alg":"standard","protocal":"standard","compress":"off"},"net_param":{"vip_v4":"10.91.0.2"}}`)
		case "/vone/login/get_config_info":
			fmt.Fprint(w, `{"Ret":0,"phone":"","mail":""}`)
		case "/vone/login/logout":
			logouts.Add(1)
			fmt.Fprint(w, `{"Ret":0}`)
		default:
			t.Error("missing resources must not start a data tunnel")
			http.Error(w, "unexpected", 400)
		}
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	ctx, cancel := context.WithCancel(context.Background())
	c := &topSAPClient{ctx: ctx, cancel: cancel, options: topsec.Options{Server: strings.TrimPrefix(server.URL, "https://"), Username: "private-test-user", Password: "private-test-password", TLSConfig: &tls.Config{RootCAs: roots}}}
	defer c.Close()
	err := c.Setup()
	if err == nil || !strings.Contains(err.Error(), "未返回完整资源列表") || logouts.Load() != 1 {
		t.Fatal("missing resources were accepted or failed session was not revoked", err)
	}
	logged, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, marker := range []string{"authentication accepted Ret=0", "endpoint=get_config_info", "Ret=0 resources_present=false resources=0", "解析服务器资源", "未返回完整资源列表", "TopSAP setup failed:", "logout completed"} {
		if !strings.Contains(string(logged), marker) {
			t.Fatalf("resource failure missing persistent marker %q", marker)
		}
	}
	for _, secret := range []string{"private-test-user", "private-test-password", "private-test-token"} {
		if strings.Contains(string(logged), secret) {
			t.Fatal("failure logs exposed credentials or session token")
		}
	}
}

func TestTopSAPCertificateVerificationRequiresProfileOptIn(t *testing.T) {
	for _, skip := range []bool{false, true} {
		t.Run(fmt.Sprint(skip), func(t *testing.T) {
			gateway, count := topSAPGateway(t, false)
			host, portText, err := net.SplitHostPort(gateway.options.Server)
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(portText)
			if err != nil {
				t.Fatal(err)
			}
			conf := configs.Default()
			conf.ServerAddress = host
			conf.ServerPort = port
			conf.Username = "test"
			conf.Password = "test"
			c := newTopSAPClient(context.Background(), conf, skip, &certificateTestUnderlay{})
			defer c.Close()
			err = c.Setup()
			if skip {
				if err != nil || count.Load() != 1 {
					t.Fatal("explicit opt-in did not connect", err)
				}
			} else {
				if err == nil || count.Load() != 0 {
					t.Fatal("credentials sent without trusted certificate")
				}
			}
		})
	}
}
