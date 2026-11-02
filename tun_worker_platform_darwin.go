package main

import (
	"EasyConnectSuite/engine"
	"context"
)

func cleanupBeforeWorker() error { _, err := engine.CleanupStaleDarwinTUN(false); return err }
func startWorkerIngress(spec engine.TUNSpec, backend engine.TUNBackend, progress func(string, string)) (workerIngress, error) {
	return engine.StartDarwinTUNIngress(context.Background(), spec, backend)
}
