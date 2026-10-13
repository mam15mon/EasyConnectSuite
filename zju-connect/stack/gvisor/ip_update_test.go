package gvisor

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/client"
	"gvisor.dev/gvisor/pkg/tcpip/header"
)

type changingIPClient struct {
	client.Client
	ip     net.IP
	update func(net.IP) error
}

func (c *changingIPClient) IP() (net.IP, error)                     { return c.ip, nil }
func (c *changingIPClient) SetIPUpdateHandler(f func(net.IP) error) { c.update = f }

type udpEchoWire struct {
	s       *Stack
	packets chan []byte
}

func (w *udpEchoWire) Close() error             { return nil }
func (w *udpEchoWire) Read([]byte) (int, error) { return 0, io.EOF }
func (w *udpEchoWire) Write(packet []byte) (int, error) {
	w.packets <- append([]byte(nil), packet...)
	reply := append([]byte(nil), packet...)
	copy(reply[12:16], packet[16:20])
	copy(reply[16:20], packet[12:16])
	copy(reply[20:22], packet[22:24])
	copy(reply[22:24], packet[20:22])
	reply[26], reply[27] = 0, 0
	h := header.IPv4(reply)
	h.SetChecksum(0)
	h.SetChecksum(^h.CalculateChecksum())
	p := makeInboundPacketBuffer(reply, len(reply))
	w.s.endpoint.dispatcher.DeliverNetworkPacket(header.IPv4ProtocolNumber, p)
	p.DecRef()
	return len(packet), nil
}
func TestUpdatedClientIPCarriesUDPRoundTrip(t *testing.T) {
	c := &changingIPClient{ip: net.IP{10, 90, 0, 2}}
	s, err := NewOwnedStack(c, func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	wire := &udpEchoWire{s: s, packets: make(chan []byte, 2)}
	s.endpoint.l3Conn = wire
	if c.update == nil {
		t.Fatal("IP update handler not registered")
	}
	for _, ip := range []net.IP{{10, 90, 0, 2}, {10, 90, 0, 9}} {
		if err := c.update(ip); err != nil {
			t.Fatal(err)
		}
		conn, err := s.DialUDP(context.Background(), &net.UDPAddr{IP: net.IP{192, 0, 2, 10}, Port: 5353})
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(time.Second))
		payload := []byte("changed-IP-round-trip")
		if _, err := conn.Write(payload); err != nil {
			t.Fatal(err)
		}
		b := make([]byte, 100)
		n, err := conn.Read(b)
		conn.Close()
		if err != nil || !bytes.Equal(b[:n], payload) {
			t.Fatalf("UDP round trip: %q %v", b[:n], err)
		}
		select {
		case packet := <-wire.packets:
			if !net.IP(packet[12:16]).Equal(ip) {
				t.Fatalf("source IP=%s want=%s", net.IP(packet[12:16]), ip)
			}
		case <-time.After(time.Second):
			t.Fatal("no packet sent")
		}
	}
}
