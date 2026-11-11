//go:build linux && !android

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func prepareTopSAPDeviceTest(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	// This fixture replaces only the authorization boundary. Production has
	// no identity setting; native hardware/algorithm tests run separately.
	script := "#!/bin/sh\n[ \"$#\" -eq 2 ] && [ \"$1\" = \"$ECS_TEST_TOPSEC_EXE\" ] && [ \"$2\" = --topsec-device-code ] || exit 72\nprintf '%s\\n' '9ZgAH2EHpqCdTiEgY662wA=='\n"
	if err := os.WriteFile(filepath.Join(directory, "pkexec"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ECS_TEST_TOPSEC_EXE", executable)
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}
