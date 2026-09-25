// Package lifecycle exposes the terminal-hook registry (normally buried
// in internal/hook_func) to embedders of zju-connect, such as the
// EasyConnect Suite GUI, which run the engine in-process.
package lifecycle

import (
	"context"

	"github.com/mythologyli/zju-connect/internal/hook_func"
)

// TerminalFunc is a cleanup hook executed once on engine shutdown.
type TerminalFunc = hook_func.TerminalFunc

// RegisterTerminalFunc registers a cleanup hook for engine shutdown.
func RegisterTerminalFunc(name string, f TerminalFunc) {
	hook_func.RegisterTerminalFunc(name, f)
}

// ExecTerminalFunc runs all registered cleanup hooks exactly once.
func ExecTerminalFunc(ctx context.Context) []error {
	return hook_func.ExecTerminalFunc(ctx)
}

// IsTerminal reports whether shutdown has started.
func IsTerminal() bool {
	return hook_func.IsTerminal()
}

// ResetTerminalFuncs clears the registry so a fresh engine instance can
// register hooks and run again in the same process.
func ResetTerminalFuncs() {
	hook_func.ResetTerminalFuncs()
}
