package easyconnect

import (
	"io"
	"strings"
	"testing"
)

type retryTestWriter struct{ strings.Builder }

func (*retryTestWriter) Close() error { return nil }

func TestSuccessfulTrafficResetsChannelFailureBudget(t *testing.T) {
	c := &L3Conn{
		recvConn:     io.NopCloser(strings.NewReader("ok")),
		sendConn:     &retryTestWriter{},
		recvErrCount: 5,
		sendErrCount: 5,
	}
	defer c.Close()
	buf := make([]byte, 2)
	if _, err := c.Read(buf); err != nil || string(buf) != "ok" {
		t.Fatalf("receive = %q, %v", buf, err)
	}
	if _, err := c.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if c.recvErrCount != 0 || c.sendErrCount != 0 {
		t.Fatalf("successful traffic retained failure budget: receive=%d send=%d", c.recvErrCount, c.sendErrCount)
	}
}
