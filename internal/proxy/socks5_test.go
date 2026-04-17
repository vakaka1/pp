package proxy

import (
	"bytes"
	"net"
	"testing"
	"time"
)

// mockConn
type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
}

func (c *mockConn) Read(b []byte) (n int, err error)   { return c.readBuf.Read(b) }
func (c *mockConn) Write(b []byte) (n int, err error)  { return c.writeBuf.Write(b) }
func (c *mockConn) Close() error                       { return nil }
func (c *mockConn) LocalAddr() net.Addr                { return nil }
func (c *mockConn) RemoteAddr() net.Addr               { return nil }
func (c *mockConn) SetDeadline(t time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestSOCKS5(t *testing.T) {
	// Simple test for SOCKS5 handshake + request
	req := []byte{
		0x05, 0x01, 0x00, // Handshake
		0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, 0x01, 0xbb, // Request IPv4 127.0.0.1:443
	}
	conn := &mockConn{
		readBuf:  bytes.NewBuffer(req),
		writeBuf: new(bytes.Buffer),
	}

	request, err := HandleSOCKS5(conn)
	if err != nil {
		t.Fatalf("HandleSOCKS5 failed: %v", err)
	}

	if request.Target != "127.0.0.1:443" {
		t.Fatalf("unexpected target: %s", request.Target)
	}
}
