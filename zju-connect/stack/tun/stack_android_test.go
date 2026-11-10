package tun

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

type streamConn struct{ net.Conn }

func (c streamConn) ReadStream(p []byte) (int, uint64, error) { n, err := c.Read(p); return n, 0, err }

type packetDevice struct {
	packets chan []byte
	closed  chan struct{}
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
	go func() { done <- forwardPackets(streamConn{local}, device) }()
	packet := []byte{0x45, 0, 0, 20, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	go func() {
		_, _ = remote.Write(packet[:7])
		_, _ = remote.Write(append(append([]byte(nil), packet[7:]...), packet...))
	}()
	for i := 0; i < 2; i++ {
		select {
		case got := <-device.packets:
			if !bytes.Equal(got, packet) {
				t.Fatalf("packet %d: %x", i, got)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for complete packet")
		}
	}
	_ = remote.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("forwarding did not stop")
	}
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

type streamChunk struct {
	data []byte
	id   uint64
}
type scriptedStream struct{ chunks []streamChunk }

func (s *scriptedStream) ReadStream(p []byte) (int, uint64, error) {
	if len(s.chunks) == 0 {
		return 0, 0, io.EOF
	}
	chunk := &s.chunks[0]
	n := copy(p, chunk.data)
	id := chunk.id
	chunk.data = chunk.data[n:]
	if len(chunk.data) == 0 {
		s.chunks = s.chunks[1:]
	}
	return n, id, nil
}

func TestStreamPacketDiscardsPartialPacketOnReconnect(t *testing.T) {
	packet := []byte{0x45, 0, 0, 24, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2, 1, 2, 3, 4}
	for _, partial := range []int{7, 22} {
		t.Run(fmt.Sprintf("partial_%d", partial), func(t *testing.T) {
			s := &scriptedStream{chunks: []streamChunk{{packet[:partial], 1}, {append(append([]byte(nil), packet...), packet...), 2}}}
			out := make([]byte, maxInboundPacketSize)
			var pending []byte
			var id uint64
			var have bool
			for i := 0; i < 2; i++ {
				n, rest, nextID, nextHave, err := readStreamPacket(s, out, make([]byte, maxInboundPacketSize), pending, id, have)
				if err != nil || !bytes.Equal(out[:n], packet) {
					t.Fatalf("packet %d: got %x, error %v", i, out[:n], err)
				}
				pending, id, have = rest, nextID, nextHave
			}
		})
	}
}

func TestForwardPacketsReadsWholePacketProtocol(t *testing.T) {
	l3 := &packetProtocol{packet: []byte{0x45, 0, 0, 24, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2, 1, 2, 3, 4}, done: make(chan struct{})}
	device := &packetDevice{packets: make(chan []byte, 1), closed: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- forwardPackets(l3, device) }()
	select {
	case got := <-device.packets:
		if !bytes.Equal(got, l3.packet) {
			t.Fatalf("packet changed: %x", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for packet")
	}
	l3.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("forwarding did not stop")
	}
}

type packetProtocol struct {
	packet []byte
	sent   bool
	done   chan struct{}
	once   sync.Once
}

func (p *packetProtocol) Read(b []byte) (int, error) {
	if !p.sent {
		p.sent = true
		if len(b) < len(p.packet) {
			return 0, io.ErrShortBuffer
		}
		return copy(b, p.packet), nil
	}
	<-p.done
	return 0, io.EOF
}
func (p *packetProtocol) Write(b []byte) (int, error) { return len(b), nil }
func (p *packetProtocol) Close() error                { p.once.Do(func() { close(p.done) }); return nil }

func TestStreamPacketFramesIPv6AndIPv4(t *testing.T) {
	ipv6 := make([]byte, 44)
	ipv6[0], ipv6[5], ipv6[6] = 0x60, 4, 17
	copy(ipv6[40:], []byte{1, 2, 3, 4})
	ipv4 := []byte{0x45, 0, 0, 20, 0, 0, 0, 0, 64, 17, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2}
	s := &scriptedStream{chunks: []streamChunk{{ipv6[:10], 1}, {append(append([]byte(nil), ipv6[10:]...), ipv4...), 1}}}
	out, chunk := make([]byte, maxInboundPacketSize), make([]byte, maxInboundPacketSize)
	var pending []byte
	var id uint64
	var have bool
	for i, want := range [][]byte{ipv6, ipv4} {
		n, rest, nextID, nextHave, err := readStreamPacket(s, out, chunk, pending, id, have)
		if err != nil || !bytes.Equal(out[:n], want) {
			t.Fatalf("packet %d: got %x, error %v", i, out[:n], err)
		}
		pending, id, have = rest, nextID, nextHave
	}
}

func TestForwardPacketsSendsICMPIPv4(t *testing.T) {
	local, remote := net.Pipe()
	device, app := net.Pipe()
	defer remote.Close()
	defer app.Close()
	done := make(chan error, 1)
	go func() { done <- forwardPackets(local, device) }()
	packet := []byte{0x45, 0, 0, 28, 0, 0, 0, 0, 64, 1, 0, 0, 10, 0, 0, 1, 10, 0, 0, 2, 8, 0, 0, 0, 0, 1, 0, 1}
	go func() {
		ipv6 := make([]byte, 40)
		ipv6[0] = 0x60
		_, _ = app.Write(ipv6)
		_, _ = app.Write(packet)
	}()
	_ = remote.SetReadDeadline(time.Now().Add(3 * time.Second))
	got := make([]byte, len(packet))
	if _, err := io.ReadFull(remote, got); err != nil {
		t.Fatalf("ICMP packet was not forwarded: %v", err)
	}
	if !bytes.Equal(got, packet) {
		t.Fatalf("packet changed: %x", got)
	}
	_ = app.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("forwarding did not stop")
	}
}
