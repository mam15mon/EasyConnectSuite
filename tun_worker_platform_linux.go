package main

import (
	"EasyConnectSuite/engine"
	vpnlog "EasyConnectSuite/vpncore/log"
	"context"
	"errors"
	"fmt"
	"os"
)

// Non-persistent Linux TUN interfaces and their routes disappear with the owner.
func cleanupBeforeWorker() error { return nil }
func startWorkerIngress(ctx context.Context, spec engine.TUNSpec, backend engine.TUNBackend, progress func(string, string)) (workerIngress, error) {
	current, cancel, err := createLinuxWorkerIngress(ctx, spec, backend)
	if err != nil {
		return nil, err
	}
	return &linuxWorkerIngress{current: current, backend: backend, cancel: cancel}, nil
}

// IPC cancels its handler context when the reply is sent. The ingress lifetime
// is independent after successful creation, but cancellation during setup must
// stop the next DNS mutation after any already-sent request actually completes.
func createLinuxWorkerIngress(ctx context.Context, spec engine.TUNSpec, backend engine.TUNBackend) (workerIngress, context.CancelFunc, error) {
	lifetime, cancel := context.WithCancel(context.Background())
	stop := context.AfterFunc(ctx, cancel)
	current, err := engine.StartLinuxTUNIngress(lifetime, spec, backend)
	if !stop() {
		cancel()
	}
	if err == nil {
		err = errors.Join(ctx.Err(), lifetime.Err())
	}
	if err != nil {
		cancel()
		if current != nil {
			if cleanupErr := current.Close(); cleanupErr != nil {
				err = errors.Join(err, &engine.TUNCleanupError{Err: cleanupErr})
			}
		}
		return nil, nil, err
	}
	return current, cancel, nil
}

type linuxWorkerIngress struct {
	current workerIngress
	backend engine.TUNBackend
	cancel  context.CancelFunc
}

func (d *linuxWorkerIngress) UpdateSpec(ctx context.Context, spec engine.TUNSpec) (err error) {
	op := vpnlog.BeginOperation(fmt.Sprintf("[TUN pid=%d profile=%q session=%q attachment=%q policy=%d] ", os.Getpid(), spec.Profile, spec.Session, spec.Attachment, spec.Policy), "linux_tun_replace", ctx.Done())
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
	if err = ctx.Err(); err != nil {
		return err
	}
	op.Step("close_previous_interface")
	d.cancel()
	if err = d.current.Close(); err != nil {
		return err
	}
	d.current = nil
	if err = ctx.Err(); err != nil {
		return err
	}
	op.Step("create_updated_interface")
	current, cancel, err := createLinuxWorkerIngress(ctx, spec, d.backend)
	if err != nil {
		return err
	}
	d.current, d.cancel = current, cancel
	outcome = "replaced"
	return nil
}

func (d *linuxWorkerIngress) Close() error {
	d.cancel()
	if d.current == nil {
		return nil
	}
	return d.current.Close()
}
