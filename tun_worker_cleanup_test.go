//go:build windows || darwin || linux

package main

import (
	"context"
	"errors"
	"testing"

	"EasyConnectSuite/engine"
	"EasyConnectSuite/internal/tunipc"
)

type cleanupGuardIngress struct {
	err    error
	closed bool
}

func (i *cleanupGuardIngress) Close() error                    { i.closed = true; return i.err }
func (i *cleanupGuardIngress) UpdateSpec(engine.TUNSpec) error { return nil }

func TestWorkerRejectsNewAttachmentsAfterCleanupFailure(t *testing.T) {
	failure := errors.New("synthetic cleanup failure")
	first := tunipc.Ref{Profile: "first", Session: "s1", Attachment: "a1", Policy: 1}
	second := tunipc.Ref{Profile: "second", Session: "s2", Attachment: "a2", Policy: 1}
	other := &cleanupGuardIngress{}
	w := &workerServer{authenticated: true, closing: make(chan struct{}), reservations: map[string]bool{}, entries: map[string]*workerEntry{
		"a1": {spec: engine.TUNSpec{Ref: first}, ingress: &cleanupGuardIngress{err: failure}},
		"a2": {spec: engine.TUNSpec{Ref: second}, ingress: other},
	}}
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "detach", Ref: first}); !errors.Is(err, failure) {
		t.Fatalf("detach: %v", err)
	}
	for _, op := range []string{"prepare", "attach"} {
		if _, err := w.handle(context.Background(), tunipc.Message{Op: op, Ref: second}); !errors.Is(err, failure) {
			t.Fatalf("%s did not preserve failure: %v", op, err)
		}
	}
	if other.closed {
		t.Fatal("other attachment was closed")
	}
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "detach", Ref: second}); err != nil {
		t.Fatalf("other detach: %v", err)
	}
	if !other.closed {
		t.Fatal("other attachment was not cleaned up")
	}
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "shutdown"}); !errors.Is(err, failure) {
		t.Fatalf("shutdown lost failure: %v", err)
	}
}
