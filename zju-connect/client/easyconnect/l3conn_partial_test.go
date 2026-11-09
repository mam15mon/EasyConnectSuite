package easyconnect

import (
	"errors"
	"io"
	"testing"
)

var errPartialIO = errors.New("test transport failure")

type partialReader struct{}

func (partialReader) Read(p []byte) (int, error) { p[0] = 42; return 1, io.EOF }
func (partialReader) Close() error               { return nil }

type partialWriter struct{}

func (partialWriter) Write([]byte) (int, error) { return 1, errPartialIO }
func (partialWriter) Close() error              { return nil }

func TestL3PartialReadPreservesData(t *testing.T) {
	c := &L3Conn{recvConn: partialReader{}}
	defer c.Close()
	buf := make([]byte, 2)
	n, err := c.Read(buf)
	if n != 1 || err != nil || buf[0] != 42 {
		t.Fatalf("read = %d, %v, %v", n, err, buf)
	}
	if conn, _ := c.connection(false); conn != nil {
		t.Fatal("failed receive connection was retained")
	}
}

func TestL3PartialWriteReturnsErrorWithoutReplay(t *testing.T) {
	c := &L3Conn{sendConn: partialWriter{}}
	defer c.Close()
	n, err := c.Write([]byte{1, 2})
	if n != 1 || !errors.Is(err, errPartialIO) {
		t.Fatalf("write = %d, %v", n, err)
	}
}
