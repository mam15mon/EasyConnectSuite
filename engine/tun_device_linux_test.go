package engine

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"EasyConnectSuite/internal/tunipc"
)

func linuxTestSpec() TUNSpec {
	return TUNSpec{Ref: tunipc.Ref{Profile: "test", Session: "test", Attachment: "test", Policy: 1}}
}

func TestLinuxTUNRejectsInvalidAndCanceledRequests(t *testing.T) {
	if _, _, err := createLinuxTUN(context.Background(), TUNSpec{}); err == nil {
		t.Fatal("invalid spec accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := createLinuxTUN(ctx, linuxTestSpec()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
}

func TestLinuxTUNDeviceOwnership(t *testing.T) {
	if os.Getenv("ECS_LINUX_TUN_SYSTEM_TEST") != "1" {
		t.Skip("requires isolated Linux network namespace and TUN privileges")
	}
	file, name, err := createLinuxTUN(context.Background(), linuxTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := net.InterfaceByName(name); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := net.InterfaceByName(name); err == nil {
		t.Fatal("TUN interface remained after its owner closed")
	}
}
