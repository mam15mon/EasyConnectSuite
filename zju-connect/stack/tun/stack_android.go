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
		for {
			_, err := io.ReadFull(l3, buf[:20])
			if err == nil {
				headerLen := int(buf[0]&15) * 4
				packetLen := int(binary.BigEndian.Uint16(buf[2:4]))
				if buf[0]>>4 != 4 || headerLen < 20 || packetLen < headerLen || packetLen > len(buf) {
					err = fmt.Errorf("invalid inbound IPv4 packet header")
				} else if _, err = io.ReadFull(l3, buf[20:packetLen]); err == nil {
					var n int
					n, err = device.Write(buf[:packetLen])
					if err == nil && n != packetLen {
						err = io.ErrShortWrite
					}
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
