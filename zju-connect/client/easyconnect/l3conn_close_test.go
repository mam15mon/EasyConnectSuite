package easyconnect

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestL3CloseUnblocksConcurrentReadAndWrite(t *testing.T) {
	recv, recvPeer := net.Pipe()
	send, sendPeer := net.Pipe()
	defer recvPeer.Close()
	defer sendPeer.Close()
	c := &L3Conn{recvConn: recv, sendConn: send}
	done := make(chan error, 2)
	go func() { _, err := c.Read(make([]byte, 1)); done <- err }()
	go func() { _, err := c.Write([]byte{1}); done <- err }()
	time.Sleep(20 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if !errors.Is(err, net.ErrClosed) {
				t.Errorf("operation after close = %v, want net.ErrClosed", err)
			}
		case <-time.After(time.Second):
			t.Fatal("close did not unblock operation")
		}
	}
	if _, err := c.Read(make([]byte, 1)); !errors.Is(err, net.ErrClosed) {
		t.Errorf("read after close = %v", err)
	}
	if _, err := c.Write([]byte{1}); !errors.Is(err, net.ErrClosed) {
		t.Errorf("write after close = %v", err)
	}
}
