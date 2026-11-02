package engine

import (
	"errors"
	"fmt"
	"net"
	"time"

	"EasyConnectSuite/vpncore/log"
)

func validateIngressUpdate(t *tunIngress, spec TUNSpec) error {
	if t.spec.Ref.Attachment != "" {
		return errors.New("macOS TUN policy updates require a new interface")
	}
	return nil
}

func releaseIngressRoutes(t *tunIngress) error {
	if t.routes != nil {
		t.routes.mu.Lock()
		t.routes.closed = true
		t.routes.mu.Unlock()
	}
	if t.device == nil {
		return nil
	}
	op := log.BeginOperation(fmt.Sprintf("[TUN interface=%q] ", t.name), "darwin_tun_release", nil)
	outcome := "interrupted"
	var err error
	defer func() { op.Finish(outcome, err) }()
	op.Step("close_interface")
	if err = t.device.Close(); err != nil {
		outcome = "failed"
		return err
	}
	op.Step("wait_interface_removed")
	deadline := time.Now().Add(10 * time.Second)
	for {
		var interfaces []net.Interface
		interfaces, err = net.Interfaces()
		if err != nil {
			outcome = "failed"
			return err
		}
		present := false
		for _, iface := range interfaces {
			if iface.Name == t.name {
				present = true
				break
			}
		}
		if !present {
			outcome = "completed"
			return nil
		}
		if time.Now().After(deadline) {
			err = fmt.Errorf("macOS TUN interface %s did not disappear after close", t.name)
			outcome = "failed"
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}
