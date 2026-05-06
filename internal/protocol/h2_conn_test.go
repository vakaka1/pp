package protocol

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type deadlineRecordingConn struct {
	readDeadlineCalls int32
}

func (c *deadlineRecordingConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (c *deadlineRecordingConn) Write(b []byte) (int, error) { return len(b), nil }
func (c *deadlineRecordingConn) Close() error                { return nil }
func (c *deadlineRecordingConn) LocalAddr() net.Addr         { return &net.TCPAddr{} }
func (c *deadlineRecordingConn) RemoteAddr() net.Addr        { return &net.TCPAddr{} }
func (c *deadlineRecordingConn) SetDeadline(time.Time) error { return nil }
func (c *deadlineRecordingConn) SetReadDeadline(time.Time) error {
	atomic.AddInt32(&c.readDeadlineCalls, 1)
	return nil
}
func (c *deadlineRecordingConn) SetWriteDeadline(time.Time) error { return nil }

func TestH2StreamReadLoopDoesNotInstallIdleReadDeadline(t *testing.T) {
	conn := &deadlineRecordingConn{}
	_ = NewH2Stream(conn)

	time.Sleep(10 * time.Millisecond)

	if got := atomic.LoadInt32(&conn.readDeadlineCalls); got != 0 {
		t.Fatalf("H2 read loop installed read deadline %d times", got)
	}
}
