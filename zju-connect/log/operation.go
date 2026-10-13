package log

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const operationStallThreshold = 30 * time.Second
const diagnosticStackLimit = 256 * 1024

var nextOperation atomic.Uint64
var diagnosticSnapshots struct {
	sync.Mutex
	last time.Time
	id   uint64
}

// Operation records progress without changing network deadlines or recovery.
// Prefix and stage names must be local labels, never authentication data.
type Operation struct {
	id         uint64
	prefix     string
	name       string
	cancelled  <-chan struct{}
	started    time.Time
	mu         sync.Mutex
	stage      string
	stageSince time.Time
	done       chan struct{}
	watchDone  chan struct{}
	finishOnce sync.Once
}

func BeginOperation(prefix, name string, cancelled <-chan struct{}) *Operation {
	return beginOperation(prefix, name, cancelled, operationStallThreshold, 5*time.Second)
}

func beginOperation(prefix, name string, cancelled <-chan struct{}, threshold, poll time.Duration) *Operation {
	now := time.Now()
	o := &Operation{id: nextOperation.Add(1), prefix: prefix, name: name, cancelled: cancelled,
		started: now, stage: "begin", stageSince: now, done: make(chan struct{}), watchDone: make(chan struct{})}
	o.write("phase=begin")
	go o.watch(threshold, poll)
	return o
}

func (o *Operation) write(format string, args ...any) {
	Printf("%soperation=%d name=%s %s", o.prefix, o.id, o.name, fmt.Sprintf(format, args...))
}

func (o *Operation) Step(stage string) {
	now := time.Now()
	o.mu.Lock()
	previous, since := o.stage, o.stageSince
	o.stage, o.stageSince = stage, now
	o.mu.Unlock()
	o.write("phase=advance previous=%s previous_elapsed=%s stage=%s", previous, now.Sub(since).Round(time.Millisecond), stage)
}

func (o *Operation) isCancelled() bool {
	select {
	case <-o.cancelled:
		return true
	default:
		return false
	}
}

// Finish must run before the owning log capture is closed, including failures.
func (o *Operation) Finish(outcome string, err error) {
	o.finishOnce.Do(func() {
		close(o.done)
		<-o.watchDone
		o.mu.Lock()
		stage, since := o.stage, o.stageSince
		o.mu.Unlock()
		// HTTP URL errors may contain token-bearing queries. Retain the cause.
		var urlErr *url.Error
		for errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		var netErr net.Error
		timeout := errors.As(err, &netErr) && netErr.Timeout()
		if err != nil {
			outcome = "failed"
		}
		o.write("phase=end outcome=%s stage=%s elapsed=%s stage_elapsed=%s canceled=%t timeout=%t error_type=%T cause=%v",
			outcome, stage, time.Since(o.started).Round(time.Millisecond), time.Since(since).Round(time.Millisecond), o.isCancelled(), timeout, err, err)
	})
}

func (o *Operation) watch(threshold, poll time.Duration) {
	defer close(o.watchDone)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	cancelled := o.cancelled
	reported := false
	for {
		select {
		case <-o.done:
			return
		case <-cancelled:
			o.mu.Lock()
			stage := o.stage
			o.mu.Unlock()
			o.write("phase=cancel_requested stage=%s", stage)
			// Keep observing until Finish: cancellation itself may be ignored
			// by the blocked operation, which is evidence we need to preserve.
			cancelled = nil
		case now := <-ticker.C:
			o.mu.Lock()
			stage, since := o.stage, o.stageSince
			o.mu.Unlock()
			if reported || now.Sub(since) < threshold {
				continue
			}
			reported = true
			o.write("phase=pending stage=%s stage_elapsed=%s canceled=%t evidence=no_stage_progress root_cause=undetermined",
				stage, now.Sub(since).Round(time.Millisecond), o.isCancelled())
			o.snapshot()
		}
	}
}

func (o *Operation) snapshot() {
	diagnosticSnapshots.Lock()
	if time.Since(diagnosticSnapshots.last) < time.Minute {
		id := diagnosticSnapshots.id
		diagnosticSnapshots.Unlock()
		o.write("phase=stack_rate_limited snapshot=%d", id)
		return
	}
	diagnosticSnapshots.last = time.Now()
	diagnosticSnapshots.id++
	id := diagnosticSnapshots.id
	diagnosticSnapshots.Unlock()
	buf := make([]byte, diagnosticStackLimit)
	n := runtime.Stack(buf, true)
	o.write("phase=stack_begin snapshot=%d bytes=%d truncated=%t", id, n, n == len(buf))
	for _, line := range strings.Split(string(buf[:n]), "\n") {
		if line != "" {
			o.write("snapshot=%d stack=%s", id, stackEvidenceLine(line))
		}
	}
	o.write("phase=stack_end snapshot=%d", id)
}

// Preserve goroutine states, function names and source lines, but never emit
// function argument words/addresses from runtime.Stack.
func stackEvidenceLine(line string) string {
	if !strings.HasPrefix(line, "\t") {
		if index := strings.LastIndex(line, "("); index >= 0 {
			return line[:index] + "(...)"
		}
	}
	return line
}
