package hook_func

// ResetTerminalFuncs clears the global terminal registry and the terminal
// flag so a fresh engine instance can register its own cleanup hooks and
// run again in the same process. Only meaningful for library embedders
// (the EasyConnect Suite GUI); the standalone CLI never resets.
func ResetTerminalFuncs() {
	terminalMu.Lock()
	defer terminalMu.Unlock()

	terminalFuncList = nil
	terminalBegin = false
}
