package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mythologyli/zju-connect/client"
)

func TestTUNWaitStagesAndCleanupPersistWithIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engine.log")
	e, _, _ := recoveryTestEngine(t)
	e.opts.LogFile = path
	e.startLogCapture()
	defer e.stopLogCapture()
	e.updateTUN(&connectionPolicy{resourceIPs: []client.IPResource{}, resourceDomains: client.DomainResources{}}, false)
	e.registerTerminal("synthetic", func(context.Context) error { return nil })
	e.Stop()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`profile="recovery"`, "name=tun_update", "stage=wait_tun_operation_lock", "stage=wait_backend_policy_lock", "stage=worker_update_reply", "outcome=updated", "policy=", "name=shutdown", "name=cleanup_synthetic", "stage_elapsed=", "error_type="} {
		if !strings.Contains(string(data), marker) {
			t.Errorf("missing durable diagnostic %q", marker)
		}
	}
}
