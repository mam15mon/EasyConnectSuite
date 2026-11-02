package engine

import "errors"

func validateIngressUpdate(*tunIngress, TUNSpec) error { return nil }

func releaseIngressRoutes(t *tunIngress) error {
	var err error
	if t.routes != nil {
		err = errors.Join(err, t.routes.close())
	}
	if t.device != nil {
		err = errors.Join(err, t.device.Close())
	}
	return err
}
