package tun

import (
	"bytes"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

type packetDevice struct {
	packets chan []byte
	closed chan struct{}
}

func (d *packetDevice) Read([]byte) (int, error) { <-d.closed; return 0, io.EOF }
func (d *packetDevice) Write(p []byte) (int, error) {
	d.packets <- append([]byte(nil), p...)
	return len(p), nil
}
func (d *packetDevice) Close() error { close(d.closed); return nil }

func TestForwardPacketsFramesFragmentedAndCoalescedIPv4(t *testing.T) {
	local, remote := net.Pipe()
	device := &packetDevice{packets: make(chan []byte, 3), closed: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- forwardPackets(local, device) }()
	packet := []byte{0x45, 0, 0, 20, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	go func() {
		_, _ = remote.Write(packet[:7])
		_, _ = remote.Write(append(append([]byte(nil), packet[7:]...), packet...))
	}()
	for i := 0; i < 2; i++ {
		select {
		case got := <-device.packets:
			if !bytes.Equal(got, packet) { t.Fatalf("packet %d: %x", i, got) }
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for complete packet")
		}
	}
	_ = remote.Close()
	select { case <-done: case <-time.After(3 * time.Second): t.Fatal("forwarding did not stop") }
}

func TestForwardPacketsStopsWhenServerDisconnects(t *testing.T) {
	local, remote := net.Pipe()
	device, app := net.Pipe()
	defer app.Close()
	done := make(chan error, 1)
	go func() { done <- forwardPackets(local, device) }()
	remote.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("server disconnect did not unblock device read")
	}
}

func TestForwardPacketsTransfersBothDirections(t *testing.T) {
	local, remote := net.Pipe()
	device, app := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- forwardPackets(local, device) }()
	packet := []byte{0x45, 0, 0, 20, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	for _, pair := range [][2]net.Conn{{app, remote}, {remote, app}} {
		written := make(chan error, 1)
		go func() { _, err := pair[0].Write(packet); written <- err }()
		got := make([]byte, len(packet))
		pair[1].SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, err := io.ReadFull(pair[1], got); err != nil {
			t.Fatal(err)
		}
		if string(got) != string(packet) {
			t.Fatal("forwarded packet changed")
		}
		if err := <-written; err != nil {
			t.Fatal(err)
		}
	}
	app.Close()
	remote.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("device close did not stop forwarding")
	}
}

func TestServerDisconnectUnblocksDuplicatedDeviceDescriptor(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	original := os.NewFile(uintptr(fds[0]), "original")
	peer := os.NewFile(uintptr(fds[1]), "peer")
	defer original.Close()
	defer peer.Close()
	stack := &Stack{endpoint: &Endpoint{}}
	if err := stack.SetupTun(fds[0]); err != nil {
		t.Fatal(err)
	}
	local, remote := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- forwardPackets(local, stack.endpoint.readWriteCloser) }()
	remote.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("closing duplicated device did not unblock read")
	}
	if _, err := original.Write([]byte("x")); err != nil {
		t.Fatalf("original descriptor was closed: %v", err)
	}
}

func TestStopClosesTunnelCopyAndUnblocksForwarding(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	original := os.NewFile(uintptr(fds[0]), "original")
	peer := os.NewFile(uintptr(fds[1]), "peer")
	defer original.Close()
	defer peer.Close()
	stack := &Stack{endpoint: &Endpoint{}}
	if err := stack.SetupTun(fds[0]); err != nil {
		t.Fatal(err)
	}
	local, remote := net.Pipe()
	defer remote.Close()
	done := make(chan error, 1)
	go func() { done <- forwardPackets(local, stack.endpoint.readWriteCloser) }()
	if err := stack.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not unblock forwarding")
	}
	if _, err := original.Write([]byte("x")); err != nil {
		t.Fatalf("original descriptor closed: %v", err)
	}
}
