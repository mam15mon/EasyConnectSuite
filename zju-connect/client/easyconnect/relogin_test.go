package easyconnect

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
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

	"github.com/mythologyli/zju-connect/underlay"
)

func TestSetupReauthenticatesAfterShutdown(t *testing.T) {
	c, logins, expired, rejected := reloginGateway(t, "")
	if err := c.Setup(); err != nil {
		t.Fatal(err)
	}
	oldID, oldToken, oldIPConn := c.twfID, *c.token, c.requestIPConn
	expired.Store(true)
	if _, err := c.SendConn(); !errors.Is(err, ErrSangforShutdown) {
		t.Fatalf("expired tunnel error = %v", err)
	}
	if err := c.Setup(); err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 2 || c.twfID == oldID || *c.token == oldToken || c.requestIPConn == oldIPConn {
		t.Fatal("reconnect did not replace authentication, token and IP connection")
	}
	if _, err := oldIPConn.Write([]byte{0}); err == nil {
		t.Fatal("old IP connection remains open")
	}
	conn, err := c.SendConn()
	if err != nil {
		t.Fatalf("new session tunnel: %v", err)
	}
	conn.Close()

	rejected.Store(true)
	if err := c.Setup(); !errors.Is(err, ErrLoginRejected) {
		t.Fatalf("rejected reauthentication error = %v", err)
	}
	if c.token != nil || c.requestIPConn != nil {
		t.Fatal("rejected login retained the previous transport")
	}
	// A failed password flow must not leave a partially authorized ID reusable.
	if err := c.Setup(); !errors.Is(err, ErrLoginRejected) || logins.Load() != 4 {
		t.Fatalf("retry bypassed authentication: logins=%d error=%v", logins.Load(), err)
	}
}

func TestSetupPreservesInitialSuppliedSession(t *testing.T) {
	c, logins, _, _ := reloginGateway(t, "0000000000000000")
	if err := c.Setup(); err != nil {
		t.Fatal(err)
	}
	if logins.Load() != 0 || c.twfID != "0000000000000000" {
		t.Fatal("initial supplied session was unnecessarily replaced")
	}
}

// Serve HTTP authentication and Sangfor's binary IP/send handshakes on one
// local TLS port. The expired gateway returns SHUTDOWN for the original token.
func reloginGateway(t *testing.T, sessionID string) (*Client, *atomic.Int32, *atomic.Bool, *atomic.Bool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	certServer := httptest.NewTLSServer(nil)
	tlsConfig := &tls.Config{
		Certificates: certServer.TLS.Certificates,
		MinVersion:   tls.VersionTLS11,
		CipherSuites: []uint16{tls.TLS_RSA_WITH_RC4_128_SHA},
	}
	certServer.Close()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatal(err)
	}
	var logins atomic.Int32
	var expired, rejected atomic.Bool
	var mu sync.Mutex
	var connections []net.Conn
	var workers sync.WaitGroup
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections = append(connections, conn)
			mu.Unlock()
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(10 * time.Second))
				reader := bufio.NewReader(conn)
				first, err := reader.Peek(1)
				if err != nil {
					return
				}
				if first[0] == 0 || first[0] == 5 {
					packet := make([]byte, 64)
					if _, err := io.ReadFull(reader, packet); err != nil {
						return
					}
					wantID := fmt.Sprintf("%016d", logins.Load())
					if string(packet[36:52]) != wantID {
						t.Error("binary handshake used a stale session")
						return
					}
					reply := []byte{0, 0, 0, 0, 10, 90, 0, 2}
					if packet[0] == 5 {
						reply[0] = 2
						if expired.Load() && logins.Load() == 1 {
							reply[0] = 8
						}
					}
					conn.Write(reply)
					io.Copy(io.Discard, reader)
					return
				}
				request, err := http.ReadRequest(reader)
				if err != nil {
					return
				}
				defer request.Body.Close()
				body := "<Conf><Mline enable=\"0\" list=\"\"/></Conf>"
				switch request.URL.Path {
				case "/por/login_auth.csp":
					id := fmt.Sprintf("%016d", logins.Add(1))
					body = fmt.Sprintf("<Auth><TwfID>%s</TwfID><RSA_ENCRYPT_KEY>%s</RSA_ENCRYPT_KEY><RSA_ENCRYPT_EXP>65537</RSA_ENCRYPT_EXP><RndImg>0</RndImg></Auth>", id, key.N.Text(16))
				case "/por/login_psw.csp":
					io.Copy(io.Discard, request.Body)
					body = "<Auth><Result>1</Result></Auth>"
					if rejected.Load() {
						body = "<Auth><Result>0</Result></Auth>"
					}
				case "/por/conf.csp":
					if expired.Load() && logins.Load() == 1 {
						body = "<html>session expired</html>"
					}
				case "/por/update_session.csp":
					body = "<Auth><Message>success</Message><ErrorCode>1</ErrorCode></Auth>"
				default:
					t.Errorf("unexpected HTTP endpoint: %s", request.URL.Path)
				}
				if request.URL.Path != "/por/login_auth.csp" {
					wantCookie := "TWFID=" + fmt.Sprintf("%016d", logins.Load())
					if request.Header.Get("Cookie") != wantCookie {
						t.Error("HTTP request used a stale session")
					}
				}
				response := &http.Response{StatusCode: 200, ProtoMajor: 1, ProtoMinor: 1, Close: true,
					Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
				response.Write(conn)
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		<-acceptDone
		mu.Lock()
		for _, conn := range connections {
			conn.Close()
		}
		mu.Unlock()
		workers.Wait()
	})
	c := NewClient(Options{OwnedLifecycle: true, Server: listener.Addr().String(),
		Auth: AuthOptions{Username: "mock-user", Password: "mock-password"}, SessionID: sessionID,
		TestMultiLine: true, UnderlayDialer: newTestUnderlay(t, underlay.Options{AutoDetect: false})})
	t.Cleanup(c.Close)
	return c, &logins, &expired, &rejected
}
