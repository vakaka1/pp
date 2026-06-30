package protocol

import (
	"errors"
	"io"
	"testing"
)

type panicWriter struct{}

func (panicWriter) Write([]byte) (int, error) {
	panic("Write called after Handler finished")
}

type recordingWriter struct {
	calls int
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	w.calls++
	return len(b), nil
}

func TestHttpConnWriteAfterCloseReturnsClosedPipe(t *testing.T) {
	writer := &recordingWriter{}
	conn := &HttpConn{
		R: nil,
		W: writer,
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	n, err := conn.Write([]byte("data"))
	if n != 0 {
		t.Fatalf("expected no bytes written after close, got %d", n)
	}
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected closed pipe error, got %v", err)
	}
	if writer.calls != 0 {
		t.Fatalf("writer was called after close")
	}
}

func TestHttpConnWriteConvertsWriterPanicToClosedPipe(t *testing.T) {
	conn := &HttpConn{
		R: nil,
		W: panicWriter{},
	}

	n, err := conn.Write([]byte("data"))
	if n != 0 {
		t.Fatalf("expected no bytes written after writer panic, got %d", n)
	}
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected closed pipe error, got %v", err)
	}
}
