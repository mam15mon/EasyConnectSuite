package tunipc

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

func TestDarwinSocketChecksBothPeerPIDs(t *testing.T) {
	name := NewName()
	l, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	})
	accepted := make(chan net.Conn, 1)
	errs := make(chan error, 1)
	go func() {
		conn, err := l.Accept(context.Background(), uint32(os.Getpid()))
		if err != nil {
			errs <- err
			return
		}
		accepted <- conn
	}()
	conn, err := Dial(context.Background(), name, uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case peer := <-accepted:
		peer.Close()
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("accept timed out")
	}
	go func() {
		c, e := l.Accept(context.Background(), uint32(os.Getpid())+1)
		if c != nil {
			c.Close()
		}
		errs <- e
	}()
	conn2, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: name, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if err = <-errs; err == nil {
		t.Fatal("incorrect client PID accepted")
	}
	conn2.Close()
	go func() {
		c, e := l.Accept(context.Background(), uint32(os.Getpid()))
		if e != nil {
			errs <- e
			return
		}
		accepted <- c
	}()
	conn3, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: name, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	peer := <-accepted
	if err = checkPeer(conn3, uint32(os.Getpid())+1); err == nil {
		t.Fatal("incorrect server PID accepted")
	}
	conn3.Close()
	peer.Close()
}

func TestDarwinSocketCancelDoesNotPoisonNextAccept(t *testing.T) {
	l, err := Listen(NewName())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err = l.Accept(ctx, uint32(os.Getpid())); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel: %v", err)
	}
	result := make(chan net.Conn, 1)
	errs := make(chan error, 1)
	go func() {
		conn, err := l.Accept(context.Background(), uint32(os.Getpid()))
		if err != nil {
			errs <- err
			return
		}
		result <- conn
	}()
	conn, err := Dial(context.Background(), l.listener.Addr().String(), uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case peer := <-result:
		peer.Close()
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("accept timed out")
	}
	conn.Close()
}
