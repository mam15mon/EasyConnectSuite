package gvisor

import (
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/debug"
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
	stopRequested func() bool
	gvisorStack   *stack.Stack
	resolve       zcdns.LocalServer
	ipPool        *ippool.IPPool[[]clientpkg.DomainResource]

	endpoint *Endpoint
	ipMu     sync.Mutex
	ip       tcpip.Address

	// Relogin support (EasyConnect Suite fork): when the L3 tunnel dies
	// because the server dropped the session (io.EOF etc.), instead of
	// panicking the process we ask the owner to log in again, rebuild the
	// L3 connection and keep serving. If re-login fails too many times in
	// a row, we report the failure through fatalHandler and stop.
	reloginMu        sync.Mutex
	reloginFunc      func() error
	fatalHandler     func(error)
	recoveredHandler func()
	reloginFailures  int
	lastReloginTime  time.Time
	done             chan struct{}
	closeOnce        sync.Once
}

const NICID tcpip.NICID = 1
const MTU uint32 = 1400
const maxInboundPacketSize = 1500

// Network outages do not count toward the consecutive login failure limit.
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

// WritePackets is called when get packets from gVisor stack. Then it sends them to VPN server.
// It runs on gVisor-internal goroutines that no recover protects, so any
// panic here would kill the whole process — guard it (EasyConnect Suite fork).
func (ep *Endpoint) WritePackets(list stack.PacketBufferList) (written int, terr tcpip.Error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("WritePackets recovered from panic: %v\n%s", r, debug.Stack())
			written = list.Len()
			terr = nil
		}
	}()

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

				if ep.owner != nil && ep.owner.reconnectStopped() {
					return list.Len(), nil
				}

				// Server-initiated SHUTDOWN (cmd 0x08) or any other send
				// failure usually means the session is dead. Instead of
				// panicking / os.Exit like the upstream CLI, try to
				// re-login and rebuild the tunnel; drop the packet if
				// that fails (the TCP/UDP flows will recover).
				log.Printf("WritePackets: send error: %v; trying re-login", err)
				if ep.owner != nil && ep.owner.rebuildL3Conn(l3Conn, err) {
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
	return NewOwnedStack(client, hook_func.IsTerminal)
}

// NewOwnedStack isolates shutdown from other embedded VPN connections.
func NewOwnedStack(client clientpkg.Client, stopped func() bool) (*Stack, error) {
	s := &Stack{done: make(chan struct{}), stopRequested: stopped}

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

// SetupRecovered is called after a new L3 connection has been installed.
func (s *Stack) SetupRecovered(f func()) {
	s.reloginMu.Lock()
	s.recoveredHandler = f
	s.reloginMu.Unlock()
}

// Close tears the stack down: closes the L3 tunnel (which unblocks the
// Run read loop) and the gVisor netstack. Safe to call multiple times.
func (s *Stack) Close() error {
	s.closeOnce.Do(func() {
		if s.done != nil {
			close(s.done)
		}
	})
	if s.endpoint != nil {
		s.endpoint.connMu.Lock()
		conn := s.endpoint.l3Conn
		s.endpoint.l3Conn = nil
		s.endpoint.connMu.Unlock()

		safeCloseL3(conn)
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

// safeCloseL3 closes an L3 connection, converting Close-time panics into
// no-ops (utls can panic closing broken/typed-nil conns).
func safeCloseL3(conn io.ReadWriteCloser) {
	if conn == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	_ = conn.Close()
}

// dropBrokenConn detaches (and closes) the current L3 connection after an
// unexpected failure so the rebuild campaign installs a fresh one.
func (s *Stack) dropBrokenConn() {
	s.endpoint.connMu.Lock()
	conn := s.endpoint.l3Conn
	s.endpoint.l3Conn = nil
	s.endpoint.connMu.Unlock()
	safeCloseL3(conn)
}

// rebuildL3Conn tries to recover a dead L3 tunnel with a serialized
// re-login campaign: network outages wait until connectivity returns;
// other failures stop after maxReloginAttempts. Callers block on
// the mutex while one campaign runs — the tunnel is dead for them anyway.
// broken is the connection the error came from; when nil or already
// replaced, another path has rebuilt the tunnel and we are done.
// Returns true only when the tunnel is usable again; every false return
// is final (terminal shutdown or fatal already reported).
func (s *Stack) rebuildL3Conn(broken io.ReadWriteCloser, cause error) bool {
	if errors.Is(cause, easyconnect.ErrSangforShutdown) {
		log.Printf("L3 tunnel: server returned SHUTDOWN (session terminated by server)")
	}

	s.reloginMu.Lock()
	var fatalCause error
	recovered := false
	defer func() {
		recoveredHandler := s.recoveredHandler
		handler := s.fatalHandler
		s.reloginMu.Unlock()
		if recovered && recoveredHandler != nil {
			recoveredHandler()
		}
		if fatalCause != nil {
			if handler != nil {
				handler(fatalCause)
			} else {
				log.Printf("gVisor fatal: %v", fatalCause)
			}
		}
	}()

	if s.reconnectStopped() {
		return false
	}

	if s.reloginFunc == nil {
		fatalCause = cause
		return false
	}

	s.endpoint.connMu.RLock()
	current := s.endpoint.l3Conn
	s.endpoint.connMu.RUnlock()
	if current != nil && broken != nil && current != broken {
		log.Printf("L3 tunnel already rebuilt by another path, continuing")
		return true
	}

	for {
		if s.reconnectStopped() {
			return false
		}

		if s.reloginFailures >= maxReloginAttempts {
			fatalCause = fmt.Errorf("L3 tunnel dead (%v): %d consecutive re-login attempts failed, giving up", cause, s.reloginFailures)
			return false
		}

		if wait := reloginCooldown - time.Since(s.lastReloginTime); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-s.done:
				timer.Stop()
				return false
			case <-timer.C:
			}
		}
		if s.reconnectStopped() {
			return false
		}

		s.lastReloginTime = time.Now()
		log.Printf("L3 tunnel error: %v; re-login attempt %d/%d", cause, s.reloginFailures+1, maxReloginAttempts)

		if err := s.callRelogin(); err != nil {
			if errors.Is(err, easyconnect.ErrLoginRejected) {
				fatalCause = fmt.Errorf("VPN 登录被拒绝，已停止自动重连: %w", err)
				log.Printf("Re-login rejected, stopping: %v", err)
				return false
			}
			if isReconnectNetworkError(err) {
				log.Printf("Network unavailable, waiting to reconnect: %v", err)
				continue
			}
			s.reloginFailures++
			log.Printf("Re-login failed (%d/%d): %v", s.reloginFailures, maxReloginAttempts, err)
			continue
		}

		newConn, err := s.newL3ConnSafe()
		if err != nil {
			if isReconnectNetworkError(err) {
				log.Printf("L3 network unavailable, waiting to reconnect: %v", err)
				continue
			}
			s.reloginFailures++
			log.Printf("Re-login succeeded but L3 handshake failed (%d/%d): %v", s.reloginFailures, maxReloginAttempts, err)
			continue
		}
		s.endpoint.connMu.Lock()
		if s.reconnectStopped() {
			s.endpoint.connMu.Unlock()
			safeCloseL3(newConn)
			return false
		}

		oldConn := s.endpoint.l3Conn
		s.endpoint.l3Conn = newConn
		s.endpoint.connMu.Unlock()

		safeCloseL3(oldConn)

		s.reloginFailures = 0
		recovered = true
		log.Printf("Re-login success, L3 tunnel rebuilt")
		return true
	}
}

func (s *Stack) reconnectStopped() bool {
	if s.stopRequested != nil && s.stopRequested() {
		return true
	}
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func isReconnectNetworkError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// callRelogin invokes the registered re-login callback, converting a
// panic inside it into an error so one bad attempt cannot kill the
// calling read loop.
func (s *Stack) callRelogin() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("re-login panicked: %v", r)
			log.Printf("Re-login panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return s.reloginFunc()
}

// newL3ConnSafe wraps NewL3Conn with the same panic guard.
func (s *Stack) newL3ConnSafe() (conn io.ReadWriteCloser, err error) {
	defer func() {
		if r := recover(); r != nil {
			conn = nil
			err = fmt.Errorf("L3 handshake panicked: %v", r)
			log.Printf("L3 handshake panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return s.endpoint.client.NewL3Conn()
}

// Run keeps the L3 tunnel alive for the lifetime of the stack: it reads
// inbound packets until the tunnel dies (error OR panic), then runs the
// re-login campaign and starts over. It only returns when the engine is
// shutting down or the failure has been reported as fatal — it never
// exits silently.
func (s *Stack) Run() {
	for {
		if s.reconnectStopped() {
			return
		}

		err := s.readLoopOnce()

		if s.reconnectStopped() {
			return
		}
		if err == nil {
			// l3Conn cleared externally (Stack.Close); terminal hooks
			// handle the rest.
			return
		}

		if !s.rebuildL3Conn(nil, err) {
			// fatal already reported through the engine
			return
		}
	}
}

// readLoopOnce reads inbound packets from the current L3 connection until
// it fails. A recovered panic is converted into an error (and the broken
// conn detached) so an unexpected bug cannot silently kill the reader
// goroutine — which previously left the UI stuck in "connecting" forever.
func (s *Stack) readLoopOnce() (cause error) {
	defer func() {
		if r := recover(); r != nil {
			cause = fmt.Errorf("L3 read loop panicked: %v", r)
			log.Printf("gVisor read loop recovered: %v\n%s", r, debug.Stack())
			s.dropBrokenConn()
		}
	}()

	s.endpoint.connMu.RLock()
	conn := s.endpoint.l3Conn
	s.endpoint.connMu.RUnlock()

	if conn == nil {
		var err error
		conn, err = s.newL3ConnSafe()
		if err != nil {
			return fmt.Errorf("create L3 connection: %w", err)
		}
		s.endpoint.connMu.Lock()
		s.endpoint.l3Conn = conn
		s.endpoint.connMu.Unlock()
	}

	// Read from VPN server and send to gVisor stack
	buf := make([]byte, maxInboundPacketSize)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return err
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
