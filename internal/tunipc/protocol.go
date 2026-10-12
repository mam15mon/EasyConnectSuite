// Package tunipc is the private, versioned transport between the GUI and its TUN worker.
package tunipc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

const MaxFrame = 1 << 20
const MaxFlows = 1024

type Ref struct {
	Profile    string `json:"profile"`
	Session    string `json:"session"`
	Attachment string `json:"attachment"`
	Policy     uint64 `json:"policy"`
}
type Spec struct {
	Ref
	Slot     int      `json:"slot"`
	Prefixes []string `json:"prefixes"`
	Names    []string `json:"names"`
}

func (s Spec) Validate() error {
	if s.Profile == "" || s.Session == "" || s.Attachment == "" || s.Policy == 0 || s.Slot < 0 || s.Slot > 63 {
		return errors.New("invalid TUN identity")
	}
	if len(s.Prefixes) > 8192 || len(s.Names) > 8192 {
		return errors.New("too many TUN resources")
	}
	for _, text := range s.Prefixes {
		p, e := netip.ParsePrefix(text)
		if e != nil || !p.Addr().Is4() || p.Bits() == 0 || p != p.Masked() {
			return fmt.Errorf("invalid TUN resource prefix %q", text)
		}
	}
	for _, n := range s.Names {
		n = string(bytes.TrimPrefix([]byte(n), []byte(".")))
		if len(n) == 0 || len(n) > 253 {
			return errors.New("invalid TUN DNS namespace")
		}
		for _, c := range n {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
				return errors.New("invalid TUN DNS namespace")
			}
		}
	}
	return nil
}

type Message struct {
	Version int      `json:"version"`
	ID      uint64   `json:"id"`
	Kind    string   `json:"kind"`
	Op      string   `json:"op"`
	Ref     Ref      `json:"ref"`
	Spec    *Spec    `json:"spec,omitempty"`
	Network string   `json:"network,omitempty"`
	Address string   `json:"address,omitempty"`
	Pipe    string   `json:"pipe,omitempty"`
	Data    []byte   `json:"data,omitempty"`
	Routes  []string `json:"routes,omitempty"`
	Error   string   `json:"error,omitempty"`
	Text    string   `json:"text,omitempty"`
}

func ReadMessage(r io.Reader) (Message, error) {
	var m Message
	var h [4]byte
	if _, e := io.ReadFull(r, h[:]); e != nil {
		return m, e
	}
	size := binary.BigEndian.Uint32(h[:])
	if size == 0 || size > MaxFrame {
		return m, errors.New("invalid IPC frame length")
	}
	data := make([]byte, size)
	if _, e := io.ReadFull(r, data); e != nil {
		return m, e
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e := d.Decode(&m); e != nil {
		return m, e
	}
	if d.Decode(new(any)) != io.EOF {
		return m, errors.New("trailing IPC data")
	}
	if m.Version != 1 || (m.Kind != "request" && m.Kind != "reply" && m.Kind != "event") || m.Op == "" {
		return m, errors.New("invalid IPC envelope")
	}
	return m, nil
}
func writeMessage(w io.Writer, m Message) error {
	m.Version = 1
	b, e := json.Marshal(m)
	if e != nil {
		return e
	}
	if len(b) > MaxFrame {
		return errors.New("IPC frame too large")
	}
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(b)))
	return writeAll(w, append(h[:], b...))
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

type Handler func(context.Context, Message) (Message, error)
type Peer struct {
	conn     net.Conn
	handler  Handler
	event    func(Message)
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	once     sync.Once
	next     atomic.Uint64
	writeMu  sync.Mutex
	mu       sync.Mutex
	pending  map[uint64]chan Message
	requests map[uint64]context.CancelFunc
	slots    chan struct{}
}

func NewPeer(c net.Conn, h Handler, event func(Message)) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{conn: c, handler: h, event: event, ctx: ctx, cancel: cancel, done: make(chan struct{}), pending: map[uint64]chan Message{}, requests: map[uint64]context.CancelFunc{}, slots: make(chan struct{}, 64)}
	go p.read()
	return p
}
func (p *Peer) Done() <-chan struct{} { return p.done }
func (p *Peer) Close()                { p.once.Do(func() { p.cancel(); _ = p.conn.Close(); close(p.done) }) }
func (p *Peer) send(m Message) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.ctx.Err() != nil {
		return net.ErrClosed
	}
	_ = p.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if e := writeMessage(p.conn, m); e != nil {
		p.Close()
		return e
	}
	return nil
}
func (p *Peer) Emit(m Message) error { m.Kind = "event"; return p.send(m) }
func (p *Peer) Call(ctx context.Context, m Message) (Message, error) {
	m.ID = p.next.Add(1)
	m.Kind = "request"
	ch := make(chan Message, 1)
	p.mu.Lock()
	if len(p.pending) >= 64 {
		p.mu.Unlock()
		return Message{}, errors.New("too many IPC requests")
	}
	p.pending[m.ID] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, m.ID); p.mu.Unlock() }()
	if e := p.send(m); e != nil {
		return Message{}, e
	}
	select {
	case r := <-ch:
		if r.Op != m.Op || r.Ref != m.Ref {
			return r, errors.New("IPC response identity mismatch")
		}
		if r.Error != "" {
			return r, errors.New(r.Error)
		}
		return r, nil
	case <-ctx.Done():
		_ = p.Emit(Message{Op: "cancel", ID: m.ID})
		return Message{}, ctx.Err()
	case <-p.done:
		return Message{}, net.ErrClosed
	}
}
func (p *Peer) read() {
	defer p.Close()
	for {
		_ = p.conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		m, e := ReadMessage(p.conn)
		if e != nil {
			return
		}
		if m.Kind == "reply" {
			p.mu.Lock()
			ch := p.pending[m.ID]
			p.mu.Unlock()
			if ch != nil {
				select {
				case ch <- m:
				default:
				}
			}
			continue
		}
		if m.Kind == "event" {
			if m.Op == "cancel" {
				p.mu.Lock()
				c := p.requests[m.ID]
				p.mu.Unlock()
				if c != nil {
					c()
				}
			} else if p.event != nil {
				p.event(m)
			}
			continue
		}
		if m.ID == 0 {
			return
		}
		p.mu.Lock()
		_, duplicate := p.requests[m.ID]
		p.mu.Unlock()
		if duplicate {
			return
		}
		select {
		case p.slots <- struct{}{}:
		default:
			_ = p.send(Message{Kind: "reply", ID: m.ID, Op: m.Op, Ref: m.Ref, Error: "too many worker requests"})
			continue
		}
		ctx, cancel := context.WithCancel(p.ctx)
		p.mu.Lock()
		p.requests[m.ID] = cancel
		p.mu.Unlock()
		go func(m Message) {
			defer func() { cancel(); p.mu.Lock(); delete(p.requests, m.ID); p.mu.Unlock(); <-p.slots }()
			r := Message{}
			var err error
			if m.Op != "ping" {
				r, err = p.handler(ctx, m)
			}
			r.Kind = "reply"
			r.ID = m.ID
			r.Op = m.Op
			r.Ref = m.Ref
			if err != nil {
				r.Error = err.Error()
			}
			_ = p.send(r)
		}(m)
	}
}
func (p *Peer) Heartbeat() {
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-p.done:
				return
			case <-t.C:
				ctx, c := context.WithTimeout(p.ctx, 5*time.Second)
				_, e := p.Call(ctx, Message{Op: "ping"})
				c()
				if e != nil {
					p.Close()
					return
				}
			}
		}
	}()
}

// DataConn keeps UDP packet boundaries and directional TCP EOF independent of control traffic.
type DataConn struct {
	net.Conn
	packet          bool
	readMu, writeMu sync.Mutex
	remaining       []byte
	eof             bool
}

func NewDataConn(c net.Conn, packet bool) *DataConn { return &DataConn{Conn: c, packet: packet} }
func (c *DataConn) Read(b []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if c.eof {
		return 0, io.EOF
	}
	if len(c.remaining) == 0 {
		var h [4]byte
		if _, e := io.ReadFull(c.Conn, h[:]); e != nil {
			return 0, e
		}
		n := binary.BigEndian.Uint32(h[:])
		if n == 0 {
			c.eof = true
			return 0, io.EOF
		}
		if n > 65535 {
			return 0, errors.New("data frame too large")
		}
		c.remaining = make([]byte, n)
		if _, e := io.ReadFull(c.Conn, c.remaining); e != nil {
			return 0, e
		}
	}
	if c.packet && len(b) < len(c.remaining) {
		c.remaining = nil
		return 0, io.ErrShortBuffer
	}
	n := copy(b, c.remaining)
	c.remaining = c.remaining[n:]
	return n, nil
}
func (c *DataConn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.packet && len(b) > 65535 {
		return 0, errors.New("UDP datagram too large")
	}
	written := 0
	for len(b) > 0 {
		n := len(b)
		if n > 65535 {
			n = 65535
		}
		var h [4]byte
		binary.BigEndian.PutUint32(h[:], uint32(n))
		if e := writeAll(c.Conn, h[:]); e != nil {
			return written, e
		}
		if e := writeAll(c.Conn, b[:n]); e != nil {
			return written, e
		}
		written += n
		b = b[n:]
	}
	return written, nil
}
func (c *DataConn) CloseWrite() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return writeAll(c.Conn, make([]byte, 4))
}

func Relay(a, b net.Conn, packet bool) {
	var wg sync.WaitGroup
	wg.Add(2)
	copyOne := func(dst, src net.Conn) {
		defer wg.Done()
		if packet {
			buf := make([]byte, 65535)
			for {
				n, e := src.Read(buf)
				if e != nil {
					break
				}
				if _, e = dst.Write(buf[:n]); e != nil {
					break
				}
			}
			_ = dst.Close()
			_ = src.Close()
		} else {
			_, _ = io.Copy(dst, src)
			if c, ok := dst.(interface{ CloseWrite() error }); ok {
				_ = c.CloseWrite()
			} else {
				_ = dst.Close()
			}
		}
	}
	go copyOne(a, b)
	go copyOne(b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}
