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
	err     error
	closed  bool
	updated bool
}

func (i *cleanupGuardIngress) Close() error                    { i.closed = true; return i.err }
func (i *cleanupGuardIngress) UpdateSpec(engine.TUNSpec) error { i.updated = true; return nil }

func TestWorkerRejectsNewAttachmentsAfterCleanupFailure(t *testing.T) {
	failure := errors.New("synthetic cleanup failure")
	first := tunipc.Ref{Profile: "first", Session: "s1", Attachment: "a1", Policy: 1}
	second := tunipc.Ref{Profile: "second", Session: "s2", Attachment: "a2", Policy: 1}
	other := &cleanupGuardIngress{}
	third := tunipc.Ref{Profile: "third", Session: "s3", Attachment: "a3", Policy: 1}
	untouched := &cleanupGuardIngress{}
	w := &workerServer{authenticated: true, closing: make(chan struct{}), reservations: map[string]bool{}, entries: map[string]*workerEntry{
		"a1": {spec: engine.TUNSpec{Ref: first}, ingress: &cleanupGuardIngress{err: failure}},
		"a2": {spec: engine.TUNSpec{Ref: second}, ingress: other},
		"a3": {spec: engine.TUNSpec{Ref: third}, ingress: untouched},
	}}
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "detach", Ref: first}); !errors.Is(err, failure) {
		t.Fatalf("detach: %v", err)
	}
	for _, op := range []string{"prepare", "attach"} {
		if _, err := w.handle(context.Background(), tunipc.Message{Op: op, Ref: second}); !errors.Is(err, failure) {
			t.Fatalf("%s did not preserve failure: %v", op, err)
		}
	}
	updated := engine.TUNSpec{Ref: second}
	updated.Policy++
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "update", Ref: updated.Ref, Spec: &updated}); !errors.Is(err, failure) {
		t.Fatalf("update did not preserve cleanup failure: %v", err)
	}
	if other.updated {
		t.Fatal("existing attachment updated after unconfirmed cleanup")
	}
	if !other.closed || w.entries[second.Attachment] != nil {
		t.Fatal("rejected update did not retire its old attachment")
	}
	if untouched.closed {
		t.Fatal("unrelated attachment was closed")
	}
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "detach", Ref: third}); err != nil {
		t.Fatalf("other detach: %v", err)
	}
	if !untouched.closed {
		t.Fatal("other attachment was not cleaned up")
	}
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "shutdown"}); !errors.Is(err, failure) {
		t.Fatalf("shutdown lost failure: %v", err)
	}
}

type failedReplacementIngress struct{ failure error }

func (i *failedReplacementIngress) Close() error { return nil }
func (i *failedReplacementIngress) UpdateSpec(engine.TUNSpec) error {
	return errors.Join(errors.New("replacement failed"), &engine.TUNCleanupError{Err: i.failure})
}

func TestWorkerBlocksAfterReplacementCleanupFailure(t *testing.T) {
	failure := errors.New("synthetic failed attach cleanup")
	ref := tunipc.Ref{Profile: "p", Session: "s", Attachment: "a", Policy: 1}
	spec := engine.TUNSpec{Ref: ref}
	w := &workerServer{authenticated: true, closing: make(chan struct{}), reservations: map[string]bool{}, entries: map[string]*workerEntry{"a": {spec: spec, ingress: &failedReplacementIngress{failure: failure}}}}
	spec.Policy++
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "update", Ref: spec.Ref, Spec: &spec}); !errors.Is(err, failure) {
		t.Fatalf("update: %v", err)
	}
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "prepare", Ref: ref}); !errors.Is(err, failure) {
		t.Fatalf("prepare was not blocked: %v", err)
	}
	if _, err := w.handle(context.Background(), tunipc.Message{Op: "shutdown"}); !errors.Is(err, failure) {
		t.Fatalf("shutdown lost cause: %v", err)
	}
}
