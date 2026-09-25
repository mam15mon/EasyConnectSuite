package gvisor

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	clientpkg "github.com/mythologyli/zju-connect/client"
	"github.com/mythologyli/zju-connect/client/easyconnect"
	"github.com/mythologyli/zju-connect/internal/hook_func"
	"github.com/mythologyli/zju-connect/internal/ippool"
	"github.com/mythologyli/zju-connect/internal/zcdns"
	"github.com/mythologyli/zju-connect/log"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

type Stack struct {
	gvisorStack *stack.Stack
	resolve     zcdns.LocalServer
	ipPool      *ippool.IPPool[[]clientpkg.DomainResource]

	endpoint *Endpoint
	ipMu     sync.Mutex
	ip       tcpip.Address

	// Relogin support (EasyConnect Suite fork): when the L3 tunnel dies
	// because the server dropped the session (io.EOF etc.), instead of
	// panicking the process we ask the owner to log in again, rebuild the
	// L3 connection and keep serving. If re-login fails too many times in
	// a row, we report the failure through fatalHandler and stop.
	reloginMu       sync.Mutex
	reloginFunc     func() error
	fatalHandler    func(error)
	reloginFailures int
	lastReloginTime time.Time
}

const NICID tcpip.NICID = 1
const MTU uint32 = 1400
const maxInboundPacketSize = 1500

// maxReloginAttempts: give up after this many consecutive failed re-logins
const maxReloginAttempts = 5

// reloginCooldown: minimum interval between two re-login attempts
const reloginCooldown = 3 * time.Second

type Endpoint struct {
	client clientpkg.Client

	connMu sync.RWMutex
	l3Conn io.ReadWriteCloser

	// owner links back to the enclosing Stack so WritePackets can trigger
	// the re-login flow (EasyConnect Suite fork)
	owner *Stack

	dispatcher stack.NetworkDispatcher
}

func (ep *Endpoint) ParseHeader(*stack.PacketBuffer) bool {
	return true
}

func (ep *Endpoint) MTU() uint32 {
	return MTU
}

func (ep *Endpoint) SetMTU(mtu uint32) {
	log.Printf("don't support change MTU from %d to %d", MTU, mtu)
}

func (ep *Endpoint) MaxHeaderLength() uint16 {
	return 0
}

func (ep *Endpoint) LinkAddress() tcpip.LinkAddress {
	return ""
}

func (ep *Endpoint) SetLinkAddress(addr tcpip.LinkAddress) {}

func (ep *Endpoint) Capabilities() stack.LinkEndpointCapabilities {
	return stack.CapabilityNone
}

func (ep *Endpoint) Attach(dispatcher stack.NetworkDispatcher) {
	ep.dispatcher = dispatcher
}

func (ep *Endpoint) IsAttached() bool {
	return ep.dispatcher != nil
}

func (ep *Endpoint) Wait() {}

func (ep *Endpoint) ARPHardwareType() header.ARPHardwareType {
	return header.ARPHardwareNone
}

func (ep *Endpoint) AddHeader(*stack.PacketBuffer) {}

func (ep *Endpoint) Close() {}

func (ep *Endpoint) SetOnCloseAction(func()) {}

// WritePackets is called when get packets from gVisor stack. Then it sends them to VPN server
func (ep *Endpoint) WritePackets(list stack.PacketBufferList) (int, tcpip.Error) {
	for _, packetBuffer := range list.AsSlice() {
		buf := joinPacketSlices(packetBuffer.AsSlices())

		ep.connMu.RLock()
		l3Conn := ep.l3Conn
		ep.connMu.RUnlock()

		if l3Conn != nil {
			n, err := l3Conn.Write(buf)
			if err != nil {
				if errors.Is(err, clientpkg.ErrResourceNotFound) {
					log.Printf("%v", err)
					continue
				}

				if hook_func.IsTerminal() {
					return list.Len(), nil
				}

				// Server-initiated SHUTDOWN (cmd 0x08) or any other send
				// failure usually means the session is dead. Instead of
				// panicking / os.Exit like the upstream CLI, try to
				// re-login and rebuild the tunnel; drop the packet if
				// that fails (the TCP/UDP flows will recover).
				log.Printf("WritePackets: send error: %v; trying re-login", err)
				if ep.owner != nil && ep.owner.rebuildL3Conn(err) {
					continue
				}
				return list.Len(), nil
			}
			log.DebugPrintf("Send: wrote %d bytes", n)
			log.DebugDumpHex(buf[:n])
		}
	}

	return list.Len(), nil
}

func joinPacketSlices(slices [][]byte) []byte {
	total := 0
	for _, slice := range slices {
		total += len(slice)
	}
	buf := make([]byte, total)
	offset := 0
	for _, slice := range slices {
		offset += copy(buf[offset:], slice)
	}
	return buf
}

func NewStack(client clientpkg.Client) (*Stack, error) {
	s := &Stack{}

	s.gvisorStack = stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		HandleLocal:        true,
	})

	s.endpoint = &Endpoint{
		client: client,
		owner:  s,
	}

	tcpipErr := s.gvisorStack.CreateNIC(NICID, s.endpoint)
	if tcpipErr != nil {
		return nil, errors.New(tcpipErr.String())
	}

	ip, err := client.IP()
	if err != nil {
		return nil, err
	}

	addr := tcpip.AddrFromSlice(ip)
	s.ip = addr
	protoAddr := tcpip.ProtocolAddress{
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   addr,
			PrefixLen: 32,
		},
		Protocol: ipv4.ProtocolNumber,
	}

	tcpipErr = s.gvisorStack.AddProtocolAddress(NICID, protoAddr, stack.AddressProperties{})
	if tcpipErr != nil {
		return nil, errors.New(tcpipErr.String())
	}
	clientpkg.RegisterIPUpdateHandler(client, s.updateIP)

	sOpt := tcpip.TCPSACKEnabled(true)
	s.gvisorStack.SetTransportProtocolOption(tcp.ProtocolNumber, &sOpt)
	cOpt := tcpip.CongestionControlOption("cubic")
	s.gvisorStack.SetTransportProtocolOption(tcp.ProtocolNumber, &cOpt)
	s.gvisorStack.AddRoute(tcpip.Route{Destination: header.IPv4EmptySubnet, NIC: NICID})

	return s, nil
}

func (s *Stack) updateIP(ip net.IP) error {
	ip = ip.To4()
	if ip == nil {
		return errors.New("virtual IP update is not IPv4")
	}
	newAddr := tcpip.AddrFromSlice(ip)
	s.ipMu.Lock()
	defer s.ipMu.Unlock()
	if newAddr == s.ip {
		return nil
	}
	protoAddr := tcpip.ProtocolAddress{
		AddressWithPrefix: tcpip.AddressWithPrefix{Address: newAddr, PrefixLen: 32},
		Protocol:          ipv4.ProtocolNumber,
	}
	if err := s.gvisorStack.AddProtocolAddress(NICID, protoAddr, stack.AddressProperties{}); err != nil {
		return errors.New(err.String())
	}
	if err := s.gvisorStack.RemoveAddress(NICID, s.ip); err != nil {
		_ = s.gvisorStack.RemoveAddress(NICID, newAddr)
		return errors.New(err.String())
	}
	s.ip = newAddr
	return nil
}

func (s *Stack) SetupResolve(r zcdns.LocalServer) {
	s.resolve = r
}

func (s *Stack) SetupIPPool(ipPool *ippool.IPPool[[]clientpkg.DomainResource]) {
	s.ipPool = ipPool
}

// SetupRelogin registers the callback used to perform a fresh login when
// the VPN server drops the L3 session (EasyConnect Suite fork).
func (s *Stack) SetupRelogin(f func() error) {
	s.reloginMu.Lock()
	s.reloginFunc = f
	s.reloginMu.Unlock()
}

// SetupFatalHandler registers the callback invoked when the L3 tunnel
// cannot be recovered. The handler is expected to clean the engine up and
// surface the error to the user — the process must NOT be killed here
// (EasyConnect Suite fork).
func (s *Stack) SetupFatalHandler(f func(error)) {
	s.reloginMu.Lock()
	s.fatalHandler = f
	s.reloginMu.Unlock()
}

// Close tears the stack down: closes the L3 tunnel (which unblocks the
// Run read loop) and the gVisor netstack. Safe to call multiple times.
func (s *Stack) Close() error {
	if s.endpoint != nil {
		s.endpoint.connMu.Lock()
		conn := s.endpoint.l3Conn
		s.endpoint.l3Conn = nil
		s.endpoint.connMu.Unlock()

		if conn != nil {
			_ = conn.Close()
		}
	}
	s.gvisorStack.Close()
	return nil
}

// reportFatal hands a non-recoverable error to the registered handler.
func (s *Stack) reportFatal(cause error) {
	s.reloginMu.Lock()
	handler := s.fatalHandler
	s.reloginMu.Unlock()

	if handler != nil {
		handler(cause)
	} else {
		log.Printf("gVisor stack fatal error: %v", cause)
	}
}

// rebuildL3Conn tries to recover a dead L3 tunnel with a serialized
// re-login campaign: up to maxReloginAttempts login attempts with a
// cooldown between them. Callers (read loop and packet writers) block on
// the mutex while one campaign runs — the tunnel is dead for them anyway.
// Returns true only when the tunnel is usable again; every false return
// is final (terminal shutdown or fatal already reported).
func (s *Stack) rebuildL3Conn(cause error) bool {
	if errors.Is(cause, easyconnect.ErrSangforShutdown) {
		log.Printf("L3 tunnel: server returned SHUTDOWN (session terminated by server)")
	}

	s.reloginMu.Lock()
	defer s.reloginMu.Unlock()

	if hook_func.IsTerminal() {
		return false
	}

	if s.reloginFunc == nil {
		s.reportFatal(cause)
		return false
	}

	for {
		if hook_func.IsTerminal() {
			return false
		}

		if s.reloginFailures >= maxReloginAttempts {
			s.reportFatal(fmt.Errorf("L3 tunnel dead (%v): %d consecutive re-login attempts failed, giving up", cause, s.reloginFailures))
			return false
		}

		if wait := reloginCooldown - time.Since(s.lastReloginTime); wait > 0 {
			time.Sleep(wait)
		}

		s.lastReloginTime = time.Now()
		log.Printf("L3 tunnel error: %v; re-login attempt %d/%d", cause, s.reloginFailures+1, maxReloginAttempts)

		if err := s.reloginFunc(); err != nil {
			s.reloginFailures++
			log.Printf("Re-login failed (%d/%d): %v", s.reloginFailures, maxReloginAttempts, err)
			continue
		}

		newConn, err := s.endpoint.client.NewL3Conn()
		if err != nil {
			s.reloginFailures++
			log.Printf("Re-login succeeded but L3 handshake failed (%d/%d): %v", s.reloginFailures, maxReloginAttempts, err)
			continue
		}

		s.endpoint.connMu.Lock()
		oldConn := s.endpoint.l3Conn
		s.endpoint.l3Conn = newConn
		s.endpoint.connMu.Unlock()

		if oldConn != nil && oldConn != newConn {
			_ = oldConn.Close()
		}

		s.reloginFailures = 0
		log.Printf("Re-login success, L3 tunnel rebuilt")
		return true
	}
}

func (s *Stack) Run() {
	var connErr error
	conn, connErr := s.endpoint.client.NewL3Conn()
	if connErr != nil {
		if hook_func.IsTerminal() {
			return
		}
		log.Printf("Failed to create L3 connection: %v", connErr)
		s.reportFatal(connErr)
		return
	}

	s.endpoint.connMu.Lock()
	s.endpoint.l3Conn = conn
	s.endpoint.connMu.Unlock()

	// Read from VPN server and send to gVisor stack
	buf := make([]byte, maxInboundPacketSize)
	for {
		s.endpoint.connMu.RLock()
		conn := s.endpoint.l3Conn
		s.endpoint.connMu.RUnlock()
		if conn == nil {
			// Closed by Stack.Close(); terminal hooks handle the rest.
			return
		}

		n, err := conn.Read(buf)
		if err != nil {
			if hook_func.IsTerminal() {
				return
			}
			// Upstream panicked here and killed the whole process on
			// server-side disconnects. Try to re-login and keep going
			// instead (EasyConnect Suite fork).
			if s.rebuildL3Conn(err) {
				continue
			}
			return
		}
		log.DebugPrintf("Recv: read %d bytes", n)
		log.DebugDumpHex(buf[:n])

		packetBuffer := makeInboundPacketBuffer(buf, n)
		s.endpoint.dispatcher.DeliverNetworkPacket(header.IPv4ProtocolNumber, packetBuffer)
		packetBuffer.DecRef()
	}
}

func makeInboundPacketBuffer(buf []byte, n int) *stack.PacketBuffer {
	return stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(buf[:n]),
	})
}
