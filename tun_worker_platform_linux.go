package main

import (
	"EasyConnectSuite/engine"
	vpnlog "EasyConnectSuite/vpncore/log"
	"context"
	"fmt"
	"os"
)

// Non-persistent Linux TUN interfaces and their routes disappear with the owner.
func cleanupBeforeWorker() error { return nil }
func startWorkerIngress(spec engine.TUNSpec, backend engine.TUNBackend, progress func(string, string)) (workerIngress, error) {
	current, err := engine.StartLinuxTUNIngress(context.Background(), spec, backend)
	if err != nil {
		return nil, err
	}
	return &linuxWorkerIngress{current: current, backend: backend}, nil
}

type linuxWorkerIngress struct {
	current workerIngress
	backend engine.TUNBackend
}

func (d *linuxWorkerIngress) UpdateSpec(spec engine.TUNSpec) (err error) {
	op := vpnlog.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment), "linux_tun_replace", nil)
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
	current, err := engine.StartLinuxTUNIngress(context.Background(), spec, d.backend)
	if err != nil {
		return err
	}
	d.current = current
	outcome = "replaced"
	return nil
}

func (d *linuxWorkerIngress) Close() error {
	if d.current == nil {
		return nil
	}
	return d.current.Close()
}
