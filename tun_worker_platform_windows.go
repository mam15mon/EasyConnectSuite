package main

import (
	"EasyConnectSuite/engine"
	"context"
)

func cleanupBeforeWorker() error { _, err := engine.CleanupStaleTUN(false); return err }
func startWorkerIngress(ctx context.Context, spec engine.TUNSpec, backend engine.TUNBackend, progress func(string, string)) (workerIngress, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return engine.StartTUNIngress(spec, backend, progress)
}
