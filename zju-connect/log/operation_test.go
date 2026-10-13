package log

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type diagnosticOutput struct {
	mu     sync.Mutex
	data   bytes.Buffer
	events chan string
}

func (w *diagnosticOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data.Write(p)
	select {
	case w.events <- string(p):
	default:
	}
	return len(p), nil
}

func (w *diagnosticOutput) text() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.data.String()
}

func diagnosticCapture(t *testing.T) *diagnosticOutput {
	t.Helper()
	w := &diagnosticOutput{events: make(chan string, 4096)}
	previous := stdlog.Writer()
	stdlog.SetOutput(w)
	t.Cleanup(func() { stdlog.SetOutput(previous) })
	return w
}

func awaitDiagnostic(t *testing.T, w *diagnosticOutput, marker string) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line := <-w.events:
			if strings.Contains(line, marker) {
				return
			}
		case <-timer.C:
			t.Fatalf("missing diagnostic %s", marker)
		}
	}
}

// Keep a real blocked goroutine in the snapshot, not a synthetic stack string.
func waitForDiagnosticEvidence(release <-chan struct{}) { <-release }

func TestStalledOperationCapturesEvidenceAfterCancellation(t *testing.T) {
	w := diagnosticCapture(t)
	diagnosticSnapshots.Lock()
	previousSnapshot := diagnosticSnapshots.last
	diagnosticSnapshots.last = time.Time{}
	diagnosticSnapshots.Unlock()
	t.Cleanup(func() {
		diagnosticSnapshots.Lock()
		diagnosticSnapshots.last = previousSnapshot
		diagnosticSnapshots.Unlock()
	})
	release := make(chan struct{})
	defer close(release)
	go waitForDiagnosticEvidence(release)
	ctx, cancel := context.WithCancel(context.Background())
	o := beginOperation("[profile=\"synthetic%profile\"] ", "reconnect", ctx.Done(), 10*time.Millisecond, time.Millisecond)
	defer o.Finish("returned", nil)
	o.Step("wait_synthetic_lock")
	cancel()
	awaitDiagnostic(t, w, "phase=stack_end")
	o.Finish("stopped", context.Canceled)
	data := w.text()
	if !strings.Contains(data, "goroutine="+o.goroutine) || !strings.Contains(data, "goroutine "+o.goroutine+" [") {
		t.Fatal("operation caller cannot be matched to captured stack")
	}
	for _, marker := range []string{"synthetic%profile", "phase=cancel_requested", "phase=pending stage=wait_synthetic_lock", "root_cause=undetermined", "canceled=true", "[chan receive]", "waitForDiagnosticEvidence(...)", "phase=stack_begin", "truncated=false", "phase=end outcome=failed"} {
		if !strings.Contains(data, marker) {
			t.Errorf("missing %q", marker)
		}
	}
	if strings.Count(data, "phase=stack_begin") != 1 {
		t.Fatal("operation dumped more than once")
	}
	if strings.Count(data, "phase=end") != 1 {
		t.Fatal("Finish was not idempotent")
	}
	// A simultaneous stalled operation references the existing process snapshot.
	second := beginOperation("", "second", nil, time.Millisecond, time.Millisecond)
	defer second.Finish("returned", nil)
	awaitDiagnostic(t, w, "phase=stack_rate_limited")
	second.Finish("returned", nil)
	if strings.Count(w.text(), "phase=stack_begin") != 1 {
		t.Fatal("snapshot rate limit ignored")
	}
}

func TestFinishedOperationStopsWatchAndStripsTokenURL(t *testing.T) {
	w := diagnosticCapture(t)
	o := BeginOperation("[profile=\"fixture\"] ", "login", nil)
	o.Step("tls_handshake")
	err := &url.Error{Op: "Get", URL: "https://gateway.invalid/?token=PRIVATE_CANARY", Err: context.DeadlineExceeded}
	o.Finish("returned", err)
	select {
	case <-o.watchDone:
	default:
		t.Fatal("watcher survived Finish")
	}
	data := w.text()
	for _, marker := range []string{"previous_elapsed=", "stage=tls_handshake", "outcome=failed", "timeout=true", "context deadline exceeded"} {
		if !strings.Contains(data, marker) {
			t.Errorf("missing %q", marker)
		}
	}
	if strings.Contains(data, "PRIVATE_CANARY") || strings.Contains(data, "phase=stack_begin") {
		t.Fatal("token exposed or finished operation dumped")
	}
}

func TestStackEvidenceStripsArgumentsAndPreservesReceiver(t *testing.T) {
	input := "example.(*Client).authenticate(0x1234, PRIVATE_CANARY, {0x5678})"
	got := stackEvidenceLine(input)
	if got != "example.(*Client).authenticate(...)" {
		t.Fatalf("stack line = %q", got)
	}
	for _, line := range []string{"goroutine 42 [sync.Mutex.Lock]:", "\t/example/client.go:37 +0x42", "created by example.start in goroutine 1"} {
		if stackEvidenceLine(line) != line {
			t.Fatalf("lost source or wait state: %q", line)
		}
	}
}

func TestConcurrentOperationProgressAndFinish(t *testing.T) {
	w := diagnosticCapture(t)
	o := BeginOperation("", "parallel", nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); o.Step("network_wait") }()
	}
	wg.Wait()
	o.Finish("returned", errors.New("synthetic refusal"))
	if !strings.Contains(w.text(), "cause=synthetic refusal") {
		t.Fatal("failure cause lost")
	}
}
