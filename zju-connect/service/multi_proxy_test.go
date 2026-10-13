package service

import (
	"EasyConnectSuite/vpncore/dial"
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func reserveAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}
func socksGreeting(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(time.Second))
	c.Write([]byte{5, 1, 0})
	reply := make([]byte, 2)
	if _, err := io.ReadFull(c, reply); err != nil || reply[0] != 5 || reply[1] != 0 {
		c.Close()
		t.Fatal("SOCKS handshake failed", err)
	}
	return c
}
func TestSOCKSOwnedListenersCloseOnlyTheirConnections(t *testing.T) {
	a, b := reserveAddress(t), reserveAddress(t)
	d := dial.NewDialer(nil, nil, nil, false, "")
	first, err := StartSocks5(a, d, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := StartSocks5(b, d, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	c1, c2 := socksGreeting(t, a), socksGreeting(t, b)
	defer c1.Close()
	defer c2.Close()
	if _, err = StartSocks5(b, d, nil, "", ""); err == nil {
		t.Fatal("port collision silently accepted")
	}
	first.Close()
	c1.SetReadDeadline(time.Now().Add(time.Second))
	if _, err = c1.Read(make([]byte, 1)); err == nil {
		t.Fatal("owned accepted connection left open")
	}
	c3 := socksGreeting(t, b)
	c3.Close()
	c2.SetReadDeadline(time.Now().Add(40 * time.Millisecond))
	_, err = c2.Read(make([]byte, 1))
	if n, ok := err.(net.Error); !ok || !n.Timeout() {
		t.Fatal("unrelated connection was closed", err)
	}
}
func TestHTTPOwnedListenersRemainIndependent(t *testing.T) {
	a, b := reserveAddress(t), reserveAddress(t)
	d := dial.NewDialer(nil, nil, nil, false, "")
	first, err := StartHTTP(a, d)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := StartHTTP(b, d)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err = StartHTTP(b, d); err == nil {
		t.Fatal("HTTP port collision accepted")
	}
	first.Close()
	c, err := net.DialTimeout("tcp", b, time.Second)
	if err != nil {
		t.Fatal("second proxy closed", err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	io.WriteString(c, "CONNECT 127.0.0.1:1 HTTP/1.1\r\nHost: 127.0.0.1:1\r\n\r\n")
	response, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal("second proxy no longer serves", err)
	}
	response.Body.Close()
}
