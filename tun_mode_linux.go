package main

func newTUNManager() *tunManager { return newLinuxTUNManager() }

// Linux interfaces are non-persistent and owned by the Worker file descriptors.
func (a *App) cleanupTUNOnStartup() (bool, error) { return false, nil }
