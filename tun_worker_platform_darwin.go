package main

import (
	"EasyConnectSuite/engine"
	vpnlog "EasyConnectSuite/vpncore/log"
	"context"
	"fmt"
	"os"
)

func cleanupBeforeWorker() error { _, err := engine.CleanupStaleDarwinTUN(false); return err }
func startWorkerIngress(spec engine.TUNSpec, backend engine.TUNBackend, progress func(string, string)) (workerIngress, error) {
	current, err := engine.StartDarwinTUNIngress(context.Background(), spec, backend)
	if err != nil {
		return nil, err
	}
	return &darwinWorkerIngress{current: current, backend: backend}, nil
}

type darwinWorkerIngress struct {
	current workerIngress
	backend engine.TUNBackend
}

func (d *darwinWorkerIngress) UpdateSpec(spec engine.TUNSpec) (err error) {
	op := vpnlog.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment), "darwin_tun_replace", nil)
	outcome := "interrupted"
	defer func() {
		if err != nil {
			outcome = "failed"
		}
		op.Finish(outcome, err)
	}()
	if err = spec.Validate(); err != nil {
		return err
	}
	op.Step("close_previous_interface")
	if err = d.current.Close(); err != nil {
		return err
	}
	d.current = nil
	op.Step("create_updated_interface")
	current, err := engine.StartDarwinTUNIngress(context.Background(), spec, d.backend)
	if err != nil {
		return err
	}
	d.current = current
	outcome = "replaced"
	return nil
}

func (d *darwinWorkerIngress) Close() error {
	if d.current == nil {
		return nil
	}
	return d.current.Close()
}
