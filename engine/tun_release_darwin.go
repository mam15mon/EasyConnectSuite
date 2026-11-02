package engine

import "errors"

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
	if t.device != nil {
		return t.device.Close()
	}
	return nil
}
