package tunipc

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestFrameRejectsMalformed(t *testing.T) {
	for _, data := range []string{`{"version":2,"kind":"request","op":"ping"}`, `{"version":1,"kind":"request","op":"ping","secret":"x"}`, `{"version":1,"kind":"request","op":"ping"} {}`} {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(data)
		if _, e := ReadMessage(&b); e == nil {
			t.Fatal("accepted malformed frame")
		}
	}
	for _, n := range []uint32{0, MaxFrame + 1} {
		var b bytes.Buffer
		binary.Write(&b, binary.BigEndian, n)
		if _, e := ReadMessage(&b); e == nil {
			t.Fatal("accepted size")
		}
	}
	var b bytes.Buffer
	writeMessage(&b, Message{Kind: "request", Op: "dns", Data: []byte{1, 2, 3}})
	data := b.Bytes()
	if _, e := ReadMessage(bytes.NewReader(data[:len(data)-1])); e == nil {
		t.Fatal("accepted truncation")
	}
}

func TestPeerRejectsRequestOverflow(t *testing.T) {
	a, b := net.Pipe()
	entered := make(chan struct{}, 64)
	p := NewPeer(a, nil, nil)
	defer p.Close()
	q := NewPeer(b, func(ctx context.Context, _ Message) (Message, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return Message{}, ctx.Err()
	}, nil)
	defer q.Close()
	ctx, c := context.WithCancel(context.Background())
	defer c()
	var done sync.WaitGroup
	for i := 0; i < 64; i++ {
		done.Add(1)
		go func() { defer done.Done(); p.Call(ctx, Message{Op: "held"}) }()
	}
	for i := 0; i < 64; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("request never entered")
		}
	}
	if _, e := p.Call(ctx, Message{Op: "overflow"}); e == nil || e.Error() != "too many IPC requests" {
		t.Fatalf("overflow %v", e)
	}
	p.Close()
	q.Close()
	done.Wait()
}
func TestIdleControlPipeExpiresWithoutHeartbeat(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	p := NewPeer(a, nil, nil)
	defer p.Close()
	select {
	case <-p.Done():
	case <-time.After(20 * time.Second):
		t.Fatal("heartbeat loss did not close pipe")
	}
}

func TestPeerCancellationDoesNotBlockOtherRequests(t *testing.T) {
	a, b := net.Pipe()
	entered := make(chan struct{})
	canceled := make(chan struct{})
	p := NewPeer(a, nil, nil)
	defer p.Close()
	q := NewPeer(b, func(ctx context.Context, m Message) (Message, error) {
		if m.Op == "slow" {
			close(entered)
			<-ctx.Done()
			close(canceled)
			return Message{}, ctx.Err()
		}
		return Message{Data: m.Data}, nil
	}, nil)
	defer q.Close()
	ctx, c := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := p.Call(ctx, Message{Op: "slow"}); done <- e }()
	<-entered
	fast, fc := context.WithTimeout(context.Background(), time.Second)
	defer fc()
	r, e := p.Call(fast, Message{Op: "echo", Data: []byte("ok")})
	if e != nil || string(r.Data) != "ok" {
		t.Fatalf("%v %s", e, r.Data)
	}
	c()
	if !errors.Is(<-done, context.Canceled) {
		t.Fatal("missing cancel")
	}
	select {
	case <-canceled:
	case <-fast.Done():
		t.Fatal("handler still blocked")
	}
}

func TestDataDirectionalEOFAndPackets(t *testing.T) {
	for _, packet := range []bool{false, true} {
		t.Run(map[bool]string{false: "tcp", true: "udp"}[packet], func(t *testing.T) {
			a, b := net.Pipe()
			x, y := NewDataConn(a, packet), NewDataConn(b, packet)
			defer x.Close()
			defer y.Close()
			a.SetDeadline(time.Now().Add(time.Second))
			b.SetDeadline(time.Now().Add(time.Second))
			go func() {
				if packet {
					x.Write(nil)
				}
				x.Write([]byte("first"))
				x.Write([]byte("second"))
				x.CloseWrite()
			}()
			buf := make([]byte, 64)
			if packet {
				n, e := y.Read(buf)
				if n != 0 || e != nil {
					t.Fatalf("zero UDP %d %v", n, e)
				}
			}
			n, e := y.Read(buf)
			if e != nil || string(buf[:n]) != "first" {
				t.Fatalf("first %d %v", n, e)
			}
			n, e = y.Read(buf)
			if e != nil || string(buf[:n]) != "second" {
				t.Fatalf("second %d %v", n, e)
			}
			if _, e = y.Read(buf); e != io.EOF {
				t.Fatalf("EOF %v", e)
			}
			go func() { y.Write([]byte("response after EOF")); y.CloseWrite() }()
			got, e := io.ReadAll(x)
			if e != nil || string(got) != "response after EOF" {
				t.Fatalf("reverse flow %q %v", got, e)
			}
		})
	}
}
