package transport

import (
	"bytes"
	"net"
	"testing"
	"time"
)

type mockConn struct {
	net.Conn
	buf bytes.Buffer
}

func (m *mockConn) Read(p []byte) (n int, err error) { return m.buf.Read(p) }
func (m *mockConn) Write(p []byte) (n int, err error) { return m.buf.Write(p) }
func (m *mockConn) Close() error { return nil }
func (m *mockConn) LocalAddr() net.Addr { return nil }
func (m *mockConn) RemoteAddr() net.Addr { return nil }
func (m *mockConn) SetDeadline(t time.Time) error { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestTrafficShaperDistribution(t *testing.T) {
	mock := &mockConn{}
	shaper := NewShaper(mock, 30) // 30ms max jitter

	start := time.Now()
	_, err := shaper.Write([]byte("test"))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if elapsed > 100*time.Millisecond {
		t.Fatalf("jitter too high: %v", elapsed)
	}
}
