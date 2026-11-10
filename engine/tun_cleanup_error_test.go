//go:build windows || darwin || linux

package engine

import (
	"errors"
	"testing"
)

func TestTUNCleanupErrorPreservesBothCauses(t *testing.T) {
	operation := errors.New("attach failed")
	cleanup := errors.New("cleanup failed")
	if got := joinTUNCleanupError(operation, nil); got != operation {
		t.Fatalf("clean rollback changed error: %v", got)
	}
	err := joinTUNCleanupError(operation, cleanup)
	var marker *TUNCleanupError
	if !errors.Is(err, operation) || !errors.Is(err, cleanup) || !errors.As(err, &marker) {
		t.Fatalf("lost operation or cleanup cause: %v", err)
	}
}
