//go:build !darwin

package main

func setupWorkerCrashLog() error {
	setupNamedCrashLog("tun-worker-crash.log")
	return nil
}
