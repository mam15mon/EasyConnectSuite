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
	conn2, err := Dial(context.Background(), name, uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	conn2.Close()
	if err = <-errs; err == nil {
		t.Fatal("incorrect client PID accepted")
	}
	go func() {
		c, e := l.Accept(context.Background(), uint32(os.Getpid()))
		if c != nil {
			c.Close()
		}
		errs <- e
	}()
	if conn3, err := Dial(context.Background(), name, uint32(os.Getpid())+1); err == nil {
		conn3.Close()
		t.Fatal("incorrect server PID accepted")
	}
	if err = <-errs; err != nil {
		t.Fatal(err)
	}
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
	result := make(chan error, 1)
	go func() {
		conn, err := l.Accept(context.Background(), uint32(os.Getpid()))
		if conn != nil {
			conn.Close()
		}
		result <- err
	}()
	conn, err := Dial(context.Background(), l.listener.Addr().String(), uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
