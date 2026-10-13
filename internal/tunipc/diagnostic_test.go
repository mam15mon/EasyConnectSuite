package tunipc

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type ipcDiagnosticOutput struct {
	mu        sync.Mutex
	data      bytes.Buffer
	completed chan struct{}
}

func (w *ipcDiagnosticOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data.Write(p)
	if bytes.Contains(p, []byte("phase=end")) {
		w.completed <- struct{}{}
	}
	return len(p), nil
}

func TestControlDiagnosticCorrelatesCancellationWithoutPayload(t *testing.T) {
	w := &ipcDiagnosticOutput{completed: make(chan struct{}, 4)}
	previous := stdlog.Writer()
	stdlog.SetOutput(w)
	defer stdlog.SetOutput(previous)
	left, right := net.Pipe()
	p := NewPeer(left, nil, nil)
	defer p.Close()
	entered := make(chan struct{})
	q := NewPeer(right, func(ctx context.Context, m Message) (Message, error) {
		close(entered)
		<-ctx.Done()
		return Message{}, ctx.Err()
	}, nil)
	defer q.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := p.Call(ctx, Message{Op: "update", Ref: Ref{Profile: "synthetic", Session: "session", Attachment: "attachment", Policy: 2}, Data: []byte("PRIVATE_CANARY")})
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request not handled")
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-w.completed:
		case <-time.After(3 * time.Second):
			t.Fatal("missing caller or handler diagnostic completion")
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	data := w.data.String()
	for _, marker := range []string{`profile="synthetic"`, `session="session"`, "policy=2 request=1 direction=send", "policy=2 request=1 direction=handle", "stage=wait_reply", "stage=handler", "active_handlers=1", "canceled=true", "cause=context canceled"} {
		if !strings.Contains(data, marker) {
			t.Errorf("missing %q", marker)
		}
	}
	if strings.Contains(data, "PRIVATE_CANARY") {
		t.Fatal("request payload leaked")
	}
}
