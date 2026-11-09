package tun

import (
	"io"
	"net"
	"testing"
	"time"
)

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
