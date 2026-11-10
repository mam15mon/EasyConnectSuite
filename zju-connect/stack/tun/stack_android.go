package tun

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"

	"EasyConnectSuite/vpncore/client"
	"EasyConnectSuite/vpncore/internal/ippool"
	"EasyConnectSuite/vpncore/internal/zcdns"
	"EasyConnectSuite/vpncore/log"
	"golang.org/x/net/ipv4"
)

const MTU uint32 = 1400
const maxInboundPacketSize = 1500

type Stack struct {
	endpoint *Endpoint
	l3Conn   io.ReadWriteCloser
}

func (s *Stack) Run() {
	op := log.BeginOperation(fmt.Sprintf("[android pid=%d] ", os.Getpid()), "android_tun_open", nil)
	op.Step("open_data_channel")
	conn, err := s.endpoint.client.NewL3Conn()
	op.Finish("success", err)
	if err != nil {
		_ = s.endpoint.readWriteCloser.Close()
		return
	}
	log.Printf("[android pid=%d] android_tun_forward phase=begin", os.Getpid())
	err = forwardPackets(conn, s.endpoint.readWriteCloser)
	log.Printf("[android pid=%d] android_tun_forward phase=end error_type=%T", os.Getpid(), err)
}

// Both directions own neither input exclusively. Closing both on the first
// error unblocks the other direction before the session can be restarted.
func forwardPackets(l3, device io.ReadWriteCloser) error {
	var stop sync.Once
	closeBoth := func() { stop.Do(func() { _ = l3.Close(); _ = device.Close() }) }
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, maxInboundPacketSize)
		stream, isStream := l3.(interface {
			ReadStream([]byte) (int, uint64, error)
		})
		var pending []byte
		chunk := make([]byte, maxInboundPacketSize)
		var streamID uint64
		var haveStream bool
		for {
			var n int
			var err error
			if isStream {
				n, pending, streamID, haveStream, err = readStreamPacket(stream, buf, chunk, pending, streamID, haveStream)
			} else {
				n, err = l3.Read(buf)
			}
			if err == nil {
				var written int
				written, err = device.Write(buf[:n])
				if err == nil && written != n {
					err = io.ErrShortWrite
				}
			}
			if err != nil {
				closeBoth()
				done <- err
				return
			}
		}
	}()
	buf := make([]byte, MTU)
	var result error
	for {
		n, err := device.Read(buf)
		if err != nil {
			result = err
			break
		}
		header, err := ipv4.ParseHeader(buf[:n])
		if err != nil || (header.Protocol != syscall.IPPROTO_TCP && header.Protocol != syscall.IPPROTO_UDP) {
			continue
		}
		if _, err = l3.Write(buf[:n]); err != nil {
			result = err
			break
		}
	}
	closeBoth()
	readErr := <-done
	if readErr != nil && result != nil {
		log.Printf("android_tun_forward receive_error=%T send_error=%T", readErr, result)
	}
	return result
}

// Buffer bytes across reads, discarding an incomplete packet when the stream changes.
func readStreamPacket(stream interface {
	ReadStream([]byte) (int, uint64, error)
}, packet, chunk, pending []byte, streamID uint64, haveStream bool) (int, []byte, uint64, bool, error) {
	for {
		if len(pending) >= 20 {
			headerLen := int(pending[0]&15) * 4
			packetLen := int(binary.BigEndian.Uint16(pending[2:4]))
			if pending[0]>>4 != 4 || headerLen < 20 || packetLen < headerLen || packetLen > len(packet) {
				return 0, pending, streamID, haveStream, fmt.Errorf("invalid inbound IPv4 packet header")
			}
			if len(pending) >= packetLen {
				copy(packet, pending[:packetLen])
				pending = append(pending[:0], pending[packetLen:]...)
				return packetLen, pending, streamID, haveStream, nil
			}
		}
		n, id, err := stream.ReadStream(chunk[:maxInboundPacketSize-len(pending)])
		if n > 0 {
			if haveStream && id != streamID {
				if len(pending) > 0 {
					log.Printf("[android pid=%d stream=%p] inbound_stream_switch discarded_partial_bytes=%d", os.Getpid(), stream, len(pending))
				}
				pending = pending[:0]
			}
			pending = append(pending, chunk[:n]...)
			streamID, haveStream = id, true
		}
		if err != nil {
			return 0, pending, streamID, haveStream, err
		}
		if n == 0 {
			return 0, pending, streamID, haveStream, io.ErrNoProgress
		}
	}
}

type Endpoint struct {
	client client.Client

	readWriteCloser io.ReadWriteCloser
	ip              net.IP

	tcpDialer *net.Dialer
	udpDialer *net.Dialer
	configMu  sync.RWMutex
}

func (ep *Endpoint) Write(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	_, err := ep.readWriteCloser.Write(buf)
	return err
}

func (ep *Endpoint) Read(buf []byte) (int, error) {
	return ep.readWriteCloser.Read(buf)
}

func (s *Stack) AddRoute(target string) error {
	return nil
}

func (s *Stack) SetupResolve(zcdns.LocalServer) {}

func (s *Stack) SetupIPPool(*ippool.IPPool[[]client.DomainResource]) {}

func NewStack(client client.Client, _ bool, _ bool, _ []client.IPResource) (*Stack, error) {
	s := &Stack{}

	s.endpoint = &Endpoint{
		client: client,
	}

	var err error
	s.endpoint.ip, err = client.IP()
	if err != nil {
		return nil, err
	}

	// We need this dialer to bind to device otherwise packets will not be sent via TUN
	s.endpoint.tcpDialer = &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   s.endpoint.ip,
			Port: 0,
		},
	}

	s.endpoint.udpDialer = &net.Dialer{
		LocalAddr: &net.UDPAddr{
			IP:   s.endpoint.ip,
			Port: 0,
		},
	}

	return s, nil
}

func (s *Stack) Stop() error {
	return s.endpoint.readWriteCloser.Close()
}

func (s *Stack) SetupTun(fd int) error {
	duplicate, err := syscall.Dup(fd)
	if err != nil {
		return err
	}
	if err = syscall.SetNonblock(duplicate, true); err != nil {
		_ = syscall.Close(duplicate)
		return err
	}
	s.endpoint.readWriteCloser = os.NewFile(uintptr(duplicate), "tun")
	return nil
}
