package tunipc

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
)

func TestNamedPipeIdentityDeadlinesAndClose(t *testing.T) {
	ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
	defer c()
	name := NewName()
	l, e := Listen(name)
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	if other, e := Listen(name); e == nil {
		other.Close()
		t.Fatal("duplicate first instance accepted")
	}
	done := make(chan error, 1)
	go func() {
		conn, e := l.Accept(ctx, uint32(os.Getpid()))
		if e != nil {
			done <- e
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(time.Second))
		_, e = io.CopyN(conn, conn, 4)
		done <- e
	}()
	conn, e := Dial(ctx, name, uint32(os.Getpid()))
	if e != nil {
		t.Fatal(e)
	}
	conn.SetDeadline(time.Now().Add(time.Second))
	defer conn.Close()
	if _, e = conn.Write([]byte("test")); e != nil {
		t.Fatal(e)
	}
	var got [4]byte
	if _, e = io.ReadFull(conn, got[:]); e != nil || string(got[:]) != "test" {
		t.Fatalf("%s %v", got, e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}

func TestNamedPipeRejectsServerPIDAndCancelsAccept(t *testing.T) {
	l, e := Listen(NewName())
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	ctx, c := context.WithCancel(context.Background())
	c()
	if _, e = l.Accept(ctx, uint32(os.Getpid())); e == nil {
		t.Fatal("canceled accept succeeded")
	}
	l2, e := Listen(NewName())
	if e != nil {
		t.Fatal(e)
	}
	defer l2.Close()
	ctx2, c2 := context.WithTimeout(context.Background(), time.Second)
	defer c2()
	if conn, e := Dial(ctx2, l2.name, uint32(os.Getpid()+1)); e == nil {
		conn.Close()
		t.Fatal("wrong server accepted")
	}
}

func TestListenerCloseWakesPendingAccept(t *testing.T) {
	l, err := Listen(NewName())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := l.Accept(context.Background(), uint32(os.Getpid())); done <- e }()
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	for {
		l.mu.Lock()
		active := l.accepting
		l.mu.Unlock()
		if active {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("accept never entered")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("closed listener accepted")
		}
	case <-ctx.Done():
		t.Fatal("Close did not wake Accept")
	}
}
