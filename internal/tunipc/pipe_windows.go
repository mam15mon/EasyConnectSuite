package tunipc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const pipePrefix = `\\.\pipe\EasyConnectSuite-TUN-`

func NewName() string {
	var n [24]byte
	if _, e := rand.Read(n[:]); e != nil {
		panic(e)
	}
	return pipePrefix + hex.EncodeToString(n[:])
}
func validName(n string) bool {
	if !strings.HasPrefix(n, pipePrefix) {
		return false
	}
	b, e := hex.DecodeString(strings.TrimPrefix(n, pipePrefix))
	return e == nil && len(b) == 24
}

type Listener struct {
	mu           sync.Mutex
	handle       windows.Handle
	name         string
	closed       bool
	accepting    bool
	acceptCancel context.CancelFunc
	acceptDone   chan struct{}
}

func Listen(name string) (*Listener, error) {
	if !validName(name) {
		return nil, errors.New("invalid pipe name")
	}
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return nil, e
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")(A;;GA;;;BA)(A;;GA;;;SY)")
	if e != nil {
		return nil, e
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, e := windows.CreateNamedPipe(windows.StringToUTF16Ptr(name), windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED|windows.FILE_FLAG_FIRST_PIPE_INSTANCE, windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS, 1, 65536, 65536, 0, &sa)
	if e != nil {
		return nil, e
	}
	return &Listener{handle: h, name: name}, nil
}
func (l *Listener) Close() error {
	l.mu.Lock()
	l.closed = true
	if l.accepting {
		cancel, done := l.acceptCancel, l.acceptDone
		l.mu.Unlock()
		cancel()
		<-done
		return l.Close()
	}
	defer l.mu.Unlock()
	if l.handle == windows.InvalidHandle {
		return nil
	}
	e := windows.CloseHandle(l.handle)
	l.handle = windows.InvalidHandle
	return e
}
func (l *Listener) Accept(ctx context.Context, pid uint32) (net.Conn, error) {
	l.mu.Lock()
	if l.closed || l.accepting {
		l.mu.Unlock()
		return nil, net.ErrClosed
	}
	ctx, cancel := context.WithCancel(ctx)
	l.accepting = true
	l.acceptCancel = cancel
	l.acceptDone = make(chan struct{})
	h := l.handle
	l.mu.Unlock()
	defer func() { cancel(); l.mu.Lock(); l.accepting = false; close(l.acceptDone); l.mu.Unlock() }()
	if h == windows.InvalidHandle {
		return nil, net.ErrClosed
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	event, e := windows.CreateEvent(nil, 1, 0, nil)
	if e != nil {
		return nil, e
	}
	defer windows.CloseHandle(event)
	ov := windows.Overlapped{HEvent: event}
	e = windows.ConnectNamedPipe(h, &ov)
	if errors.Is(e, windows.ERROR_IO_PENDING) {
		for {
			r, we := windows.WaitForSingleObject(event, 20)
			if we != nil {
				return nil, we
			}
			if r == windows.WAIT_OBJECT_0 {
				var n uint32
				e = windows.GetOverlappedResult(h, &ov, &n, false)
				break
			}
			if ctx.Err() != nil {
				_ = windows.CancelIoEx(h, &ov)
				var n uint32
				_ = windows.GetOverlappedResult(h, &ov, &n, true)
				return nil, ctx.Err()
			}
		}
	}
	if e != nil && !errors.Is(e, windows.ERROR_PIPE_CONNECTED) {
		return nil, e
	}
	var actual uint32
	if e = windows.GetNamedPipeClientProcessId(h, &actual); e != nil {
		return nil, e
	}
	if actual != pid {
		return nil, fmt.Errorf("unexpected pipe client PID %d", actual)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.handle != h || l.closed {
		return nil, net.ErrClosed
	}
	l.handle = windows.InvalidHandle
	return newPipeConn(h, l.name)
}
func Dial(ctx context.Context, name string, pid uint32) (net.Conn, error) {
	if !validName(name) {
		return nil, errors.New("invalid pipe name")
	}
	for {
		h, e := windows.CreateFile(windows.StringToUTF16Ptr(name), windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
		if e == nil {
			var actual uint32
			e = windows.GetNamedPipeServerProcessId(h, &actual)
			if e != nil || actual != pid {
				_ = windows.CloseHandle(h)
				return nil, errors.New("pipe server identity mismatch")
			}
			return newPipeConn(h, name)
		}
		if !errors.Is(e, windows.ERROR_PIPE_BUSY) && !errors.Is(e, windows.ERROR_FILE_NOT_FOUND) {
			return nil, e
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Go 1.25 pipe File.Read and File.Write share an unsynchronized offset.
// Separate handles give each direction its own File state while retaining full duplex IO.
type pipeConn struct {
	reader, writer *os.File
	name           string
}

func newPipeConn(h windows.Handle, name string) (net.Conn, error) {
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), h, windows.CurrentProcess(), &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return &pipeConn{reader: os.NewFile(uintptr(h), name), writer: os.NewFile(uintptr(duplicate), name), name: name}, nil
}
func (c *pipeConn) Read(b []byte) (int, error)         { return c.reader.Read(b) }
func (c *pipeConn) Write(b []byte) (int, error)        { return c.writer.Write(b) }
func (c *pipeConn) Close() error                       { return errors.Join(c.reader.Close(), c.writer.Close()) }
func (c *pipeConn) SetReadDeadline(t time.Time) error  { return c.reader.SetReadDeadline(t) }
func (c *pipeConn) SetWriteDeadline(t time.Time) error { return c.writer.SetWriteDeadline(t) }
func (c *pipeConn) SetDeadline(t time.Time) error {
	return errors.Join(c.SetReadDeadline(t), c.SetWriteDeadline(t))
}

type pipeAddr string

func (a pipeAddr) Network() string       { return "pipe" }
func (a pipeAddr) String() string        { return string(a) }
func (c *pipeConn) LocalAddr() net.Addr  { return pipeAddr(c.name) }
func (c *pipeConn) RemoteAddr() net.Addr { return pipeAddr(c.name) }
