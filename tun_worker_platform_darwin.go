package main

import (
	"EasyConnectSuite/engine"
	"context"
	"errors"
)

func cleanupBeforeWorker() error { return errors.New("macOS TUN 残留清理尚未实现") }
func startWorkerIngress(spec engine.TUNSpec, backend engine.TUNBackend, progress func(string, string)) (workerIngress, error) {
	return engine.StartDarwinTUNIngress(context.Background(), spec, backend)
}
