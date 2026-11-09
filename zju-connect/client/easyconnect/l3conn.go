package easyconnect

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"EasyConnectSuite/vpncore/log"
)

type L3Conn struct {
	easyConnectClient *Client
	stateLock         sync.Mutex
	closed            bool

	sendConn     io.WriteCloser
	sendLock     sync.Mutex
	sendErrCount int

	recvConn     io.ReadCloser
	recvLock     sync.Mutex
	recvErrCount int
}

func (c *L3Conn) connection(send bool) (io.Closer, bool) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	if c.closed {
		return nil, false
	}
	if send {
		return c.sendConn, true
	}
	return c.recvConn, true
}

func (c *L3Conn) setConnection(send bool, conn io.Closer) bool {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	if c.closed {
		return false
	}
	if send {
		if conn == nil {
			c.sendConn = nil
		} else {
			c.sendConn = conn.(io.WriteCloser)
		}
	} else {
		if conn == nil {
			c.recvConn = nil
		} else {
			c.recvConn = conn.(io.ReadCloser)
		}
	}
	return true
}

// safeClose closes c, converting a Close-time panic (e.g. utls methods on
// a typed-nil *UConn) into a no-op. The recv/send conns are assigned from
// (*tls.UConn, error) returns; a failed dial leaves a typed-nil in the
// interface, and calling Close on it panics — which used to kill the
// whole read loop right after a successful re-login.
func safeClose(c io.Closer) {
	if c == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	_ = c.Close()
}

// Read tries to keep the recv stream alive: on error it re-handshakes the
// recv connection with the current token. A non-nil return means the
// token/session itself is dead — the caller should run a full re-login.
func (c *L3Conn) Read(p []byte) (n int, err error) {
	c.recvLock.Lock()
	defer c.recvLock.Unlock()

	for {
		current, open := c.connection(false)
		if !open {
			return 0, net.ErrClosed
		}
		if current == nil {
			// Previous call left the conn nil after a failed re-dial:
			// re-dial instead of calling Read on a nil interface (that
			// panic killed the whole process on gvisor goroutines).
			newConn, dialErr := c.easyConnectClient.RecvConn()
			if dialErr != nil {
				return 0, dialErr
			}
			if !c.setConnection(false, newConn) {
				safeClose(newConn)
				return 0, net.ErrClosed
			}
			current = newConn
		}

		n, err = current.(io.ReadCloser).Read(p)
		if n > 0 && err != nil {
			log.Printf("receive stream ended after delivering data: error_type=%T", err)
			safeClose(current)
			c.setConnection(false, nil)
			c.recvErrCount = 0
			return n, nil
		}
		if err == nil {
			if n > 0 {
				c.recvErrCount = 0
			}
			return
		}
		if _, open := c.connection(false); !open {
			return n, net.ErrClosed
		}
		if c.recvErrCount >= 5 {
			return
		}

		log.Printf("Error occurred while receiving, retrying: %v", err)

		// Do handshake again and create a new recvConn
		safeClose(current)
		if !c.setConnection(false, nil) {
			return 0, net.ErrClosed
		}
		newConn, dialErr := c.easyConnectClient.RecvConn()
		if dialErr != nil {
			// Old token rejected by the server — propagate so the caller
			// triggers the re-login campaign.
			return 0, dialErr
		}
		if !c.setConnection(false, newConn) {
			safeClose(newConn)
			return 0, net.ErrClosed
		}
		c.recvErrCount++

		// loop: actually retry the read on the fresh connection
	}
}

// Write mirrors Read for the send stream.
func (c *L3Conn) Write(p []byte) (n int, err error) {
	c.sendLock.Lock()
	defer c.sendLock.Unlock()

	for {
		current, open := c.connection(true)
		if !open {
			return 0, net.ErrClosed
		}
		if current == nil {
			// Previous call left the conn nil after a failed re-dial
			// (network down): re-dial, and on failure propagate the
			// error so the caller runs the re-login campaign. Calling
			// Write on the nil interface here is what crashed the app.
			newConn, dialErr := c.easyConnectClient.SendConn()
			if dialErr != nil {
				return 0, dialErr
			}
			if !c.setConnection(true, newConn) {
				safeClose(newConn)
				return 0, net.ErrClosed
			}
			current = newConn
		}

		n, err = current.(io.WriteCloser).Write(p)
		if n > 0 && err != nil {
			log.Printf("send stream failed after partial write: error_type=%T", err)
			return n, err
		}
		if err == nil {
			if n > 0 {
				c.sendErrCount = 0
			}
			return
		}
		if _, open := c.connection(true); !open {
			return n, net.ErrClosed
		}
		if c.sendErrCount >= 5 {
			return
		}

		log.Printf("Error occurred while sending, retrying: %v", err)

		// Do handshake again and create a new sendConn
		safeClose(current)
		if !c.setConnection(true, nil) {
			return 0, net.ErrClosed
		}
		newConn, dialErr := c.easyConnectClient.SendConn()
		if dialErr != nil {
			return 0, dialErr
		}
		if !c.setConnection(true, newConn) {
			safeClose(newConn)
			return 0, net.ErrClosed
		}
		c.sendErrCount++
	}
}

func (c *L3Conn) Close() error {
	op := log.BeginOperation(fmt.Sprintf("[easyconnect_l3 pid=%d conn=%p] ", os.Getpid(), c), "l3_close", nil)
	defer op.Finish("success", nil)
	op.Step("acquire_state_lock")
	c.stateLock.Lock()
	if c.closed {
		c.stateLock.Unlock()
		return nil
	}
	c.closed = true
	send, recv := c.sendConn, c.recvConn
	c.sendConn = nil
	c.recvConn = nil
	c.stateLock.Unlock()
	op.Step("close_send")
	safeClose(send)
	op.Step("close_receive")
	safeClose(recv)
	return nil
}

func (c *Client) NewL3Conn() (io.ReadWriteCloser, error) {
	conn := &L3Conn{
		easyConnectClient: c,
		sendErrCount:      0,
		recvErrCount:      0,
	}

	var err error
	conn.sendConn, err = c.SendConn()
	if err != nil {
		log.Printf("Error occurred while creating sendConn: %v", err)
		safeClose(conn.sendConn)
		return nil, err
	}

	conn.recvConn, err = c.RecvConn()
	if err != nil {
		log.Printf("Error occurred while creating recvConn: %v", err)
		safeClose(conn.sendConn)
		safeClose(conn.recvConn)
		return nil, err
	}
	return conn, nil
}
