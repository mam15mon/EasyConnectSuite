package topsec

import (
	"bytes"
	"encoding/base64"
	"log"
	"strings"
	"testing"
)

func TestDarwinDeviceCodeSynthetic(t *testing.T) {
	if got := darwinDeviceCode("abc"); got != "kAFQmDzST7DWlj99KOF/cg==" {
		t.Fatalf("synthetic identity digest: %q", got)
	}
}

func TestDarwinDeviceIdentityReadsPlatformWithoutLoggingIt(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	code, err := localDeviceCode("synthetic-profile")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(code)
	if err != nil || len(decoded) != 16 {
		t.Fatalf("invalid device digest: %v", err)
	}
	if strings.Contains(output.String(), code) || !strings.Contains(output.String(), "darwin_device_identity") {
		t.Fatal("device diagnostic missing or identity exposed")
	}
}
