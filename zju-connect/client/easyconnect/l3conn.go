package easyconnect

import (
	"io"
	"sync"

	"github.com/mythologyli/zju-connect/log"
)

type L3Conn struct {
	easyConnectClient *Client

	sendConn     io.WriteCloser
	sendLock     sync.Mutex
	sendErrCount int

	recvConn     io.ReadCloser
	recvLock     sync.Mutex
	recvErrCount int
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
		if c.recvConn == nil {
			// Previous call left the conn nil after a failed re-dial:
			// re-dial instead of calling Read on a nil interface (that
			// panic killed the whole process on gvisor goroutines).
			newConn, dialErr := c.easyConnectClient.RecvConn()
			if dialErr != nil {
				return 0, dialErr
			}
			c.recvConn = newConn
		}

		n, err = c.recvConn.Read(p)
		if err == nil {
			return
		}
		if c.recvErrCount >= 5 {
			return
		}

		log.Printf("Error occurred while receiving, retrying: %v", err)

		// Do handshake again and create a new recvConn
		safeClose(c.recvConn)
		c.recvConn = nil
		newConn, dialErr := c.easyConnectClient.RecvConn()
		if dialErr != nil {
			// Old token rejected by the server — propagate so the caller
			// triggers the re-login campaign.
			return 0, dialErr
		}
		c.recvConn = newConn
		c.recvErrCount++

		// loop: actually retry the read on the fresh connection
	}
}

// Write mirrors Read for the send stream.
func (c *L3Conn) Write(p []byte) (n int, err error) {
	c.sendLock.Lock()
	defer c.sendLock.Unlock()

	for {
		if c.sendConn == nil {
			// Previous call left the conn nil after a failed re-dial
			// (network down): re-dial, and on failure propagate the
			// error so the caller runs the re-login campaign. Calling
			// Write on the nil interface here is what crashed the app.
			newConn, dialErr := c.easyConnectClient.SendConn()
			if dialErr != nil {
				return 0, dialErr
			}
			c.sendConn = newConn
		}

		n, err = c.sendConn.Write(p)
		if err == nil {
			return
		}
		if c.sendErrCount >= 5 {
			return
		}

		log.Printf("Error occurred while sending, retrying: %v", err)

		// Do handshake again and create a new sendConn
		safeClose(c.sendConn)
		c.sendConn = nil
		newConn, dialErr := c.easyConnectClient.SendConn()
		if dialErr != nil {
			return 0, dialErr
		}
		c.sendConn = newConn
		c.sendErrCount++
	}
}

func (c *L3Conn) Close() error {
	safeClose(c.sendConn)
	c.sendConn = nil
	safeClose(c.recvConn)
	c.recvConn = nil
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
