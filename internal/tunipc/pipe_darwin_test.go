package tunipc

import (
	"context"
	"errors"
	"fmt"
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

func TestDarwinWorkerIdentityChecksUIDAndExecutable(t *testing.T) {
	l, err := Listen(NewName())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		conn, e := l.accept(context.Background(), 0, func(c *net.UnixConn) error {
			pid, e := verifyProcess(c, uint32(os.Geteuid()), exe)
			if e != nil || pid != uint32(os.Getpid()) {
				return fmt.Errorf("same process: pid=%d: %w", pid, e)
			}
			if _, e = verifyProcess(c, uint32(os.Geteuid())+1, exe); e == nil {
				return errors.New("wrong UID accepted")
			}
			if _, e = verifyProcess(c, uint32(os.Geteuid()), os.DevNull); e == nil {
				return errors.New("wrong executable accepted")
			}
			return nil
		})
		if conn != nil {
			conn.Close()
		}
		result <- e
	}()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: l.listener.Addr().String(), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("identity verification timed out")
	}
}

func TestDarwinWorkerVerifiesParentSocketAndExecutable(t *testing.T) {
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
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 1)
	errs := make(chan error, 1)
	go func() {
		c, e := l.Accept(context.Background(), uint32(os.Getpid()))
		if e != nil {
			errs <- e
			return
		}
		accepted <- c
	}()
	conn, err := Dial(context.Background(), name, uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var peer net.Conn
	select {
	case peer = <-accepted:
	case err = <-errs:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("accept timeout")
	}
	defer peer.Close()
	if err := VerifyParent(conn, name, uint32(os.Getpid()), exe); err != nil {
		t.Fatal(err)
	}
	if err := VerifyParent(conn, name, uint32(os.Getpid())+1, exe); err == nil {
		t.Fatal("wrong parent PID accepted")
	}
	if err := VerifyParent(conn, name, uint32(os.Getpid()), os.DevNull); err == nil {
		t.Fatal("wrong parent executable accepted")
	}
}
