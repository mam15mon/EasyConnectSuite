package engine

import (
	"io"
	stdlog "log"
	"strings"
	"sync"
)

// The upstream logger is process-wide; its sink must outlive every VPN using it.
var engineLogs = logCapture{engines: make(map[*Engine]bool), files: make(map[string]*rotatingWriter)}

type logCapture struct {
	mu       sync.Mutex
	engines  map[*Engine]bool
	files    map[string]*rotatingWriter
	previous io.Writer
	once     sync.Once
}

func (h *logCapture) Write(p []byte) (int, error) {
	h.mu.Lock()
	var callbacks []func(string)
	for e := range h.engines {
		if e.opts.OnLog != nil {
			callbacks = append(callbacks, e.opts.OnLog)
		}
	}
	if len(h.files) == 0 && h.previous != nil {
		_, _ = h.previous.Write(p)
	}
	for _, w := range h.files {
		_, _ = w.Write(p)
	}
	h.mu.Unlock()
	for _, cb := range callbacks {
		cb(strings.TrimRight(string(p), "\r\n"))
	}
	return len(p), nil
}
func (e *Engine) startLogCapture() {
	engineLogs.once.Do(func() { engineLogs.previous = stdlog.Writer(); stdlog.SetOutput(&engineLogs) })
	engineLogs.mu.Lock()
	defer engineLogs.mu.Unlock()
	engineLogs.engines[e] = true
	if path := e.opts.LogFile; path != "" && engineLogs.files[path] == nil {
		if w, err := newRotatingWriter(path, engineLogLimit); err == nil {
			engineLogs.files[path] = w
		}
	}
}
func (e *Engine) stopLogCapture() {
	engineLogs.mu.Lock()
	defer engineLogs.mu.Unlock()
	delete(engineLogs.engines, e)
	if len(engineLogs.engines) == 0 {
		for path, w := range engineLogs.files {
			_ = w.Close()
			delete(engineLogs.files, path)
		}
	}
}
