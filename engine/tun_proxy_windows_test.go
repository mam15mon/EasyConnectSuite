package engine

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/dial"
	"github.com/mythologyli/zju-connect/resolve"
	"github.com/mythologyli/zju-connect/service"
	"github.com/mythologyli/zju-connect/stack"
)

type echoVPNStack struct {
	stack.Stack
	target string
}

func (s *echoVPNStack) DialTCP(ctx context.Context, _ *net.TCPAddr) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", s.target)
}
func reserveProxyAddress(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}
func TestExistingSOCKSAndHTTPFlowsSurviveTUNHotToggle(t *testing.T) {
	echo, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer echo.Close()
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	vpn := &echoVPNStack{target: echo.Addr().String()}
	resolver := resolve.NewResolver(vpn, "", "", 30, nil, nil, false)
	defer resolver.Close()
	resources := []client.IPResource{ipRange("192.0.2.1", "192.0.2.1")}
	p := &connectionPolicy{stack: vpn, resolver: resolver, ips: resources, resourceIPs: resources, resourceDomains: client.DomainResources{}}
	p.dialer = dial.NewDialer(vpn, resolver, resources, false, "")
	engine := &Engine{id: 15, opts: Options{ConnectionID: "proxy-fixture", TUNController: &testController{}}}
	engine.ctx, engine.cancel = context.WithCancel(context.Background())
	engine.router.current.Store(p)
	engine.running.Store(true)
	defer engine.Stop()
	socksAddr, httpAddr := reserveProxyAddress(t), reserveProxyAddress(t)
	socks, e := service.StartSocks5(socksAddr, &engine.router, &engine.router, "", "")
	if e != nil {
		t.Fatal(e)
	}
	engine.registerTerminal("SOCKS fixture", func(context.Context) error { return socks.Close() })
	httpProxy, e := service.StartHTTP(httpAddr, &engine.router)
	if e != nil {
		t.Fatal(e)
	}
	engine.registerTerminal("HTTP fixture", func(context.Context) error { return httpProxy.Close() })
	sc, e := net.Dial("tcp", socksAddr)
	if e != nil {
		t.Fatal(e)
	}
	defer sc.Close()
	sc.SetDeadline(time.Now().Add(5 * time.Second))
	sc.Write([]byte{5, 1, 0})
	var greeting [2]byte
	if _, e = io.ReadFull(sc, greeting[:]); e != nil || greeting != [2]byte{5, 0} {
		t.Fatal("SOCKS greeting", e)
	}
	sc.Write([]byte{5, 1, 0, 1, 192, 0, 2, 1, 1, 187})
	var reply [10]byte
	if _, e = io.ReadFull(sc, reply[:]); e != nil || reply[1] != 0 {
		t.Fatal("SOCKS connect", e)
	}
	hc, e := net.Dial("tcp", httpAddr)
	if e != nil {
		t.Fatal(e)
	}
	defer hc.Close()
	hc.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(hc, "CONNECT 192.0.2.1:443 HTTP/1.1\r\nHost: 192.0.2.1:443\r\n\r\n")
	reader := bufio.NewReader(hc)
	r, e := http.ReadResponse(reader, &http.Request{Method: "CONNECT"})
	if e != nil || r.StatusCode != 200 {
		t.Fatal("HTTP connect", e)
	}
	for _, enabled := range []bool{true, false, true, false} {
		if e = engine.SetTUN(context.Background(), enabled); e != nil {
			t.Fatal(e)
		}
		if engine.router.current.Load() != p {
			t.Fatal("proxy policy replaced")
		}
		for _, flow := range []struct {
			writer net.Conn
			reader io.Reader
		}{{sc, sc}, {hc, reader}} {
			flow.writer.Write([]byte("continuous"))
			var got [10]byte
			if _, e = io.ReadFull(flow.reader, got[:]); e != nil || string(got[:]) != "continuous" {
				t.Fatalf("proxy broken after hot toggle=%t: %q %v", enabled, got, e)
			}
		}
	}
}
