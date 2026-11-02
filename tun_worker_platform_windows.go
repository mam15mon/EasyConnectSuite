package main

import "EasyConnectSuite/engine"

func cleanupBeforeWorker() error { _, err := engine.CleanupStaleTUN(false); return err }
func startWorkerIngress(spec engine.TUNSpec, backend engine.TUNBackend, progress func(string, string)) (workerIngress, error) {
	return engine.StartTUNIngress(spec, backend, progress)
}
