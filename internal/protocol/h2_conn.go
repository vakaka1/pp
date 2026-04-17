package protocol

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

type H2Stream struct {
	conn       net.Conn
	framer     *http2.Framer
	writeMut   sync.Mutex
	readBuf    bytes.Buffer
	readCond   *sync.Cond
	closed     bool
	readErr    error

	sendCond     *sync.Cond
	connWindow   int32
	streamWind   int32
	ActiveStream uint32
}

func NewH2Stream(conn net.Conn) *H2Stream {
	s := &H2Stream{
		conn:         conn,
		framer:       http2.NewFramer(conn, conn),
		connWindow:   65535,
		streamWind:   65535,
		ActiveStream: 1,
	}
	s.readCond = sync.NewCond(&sync.Mutex{})
	s.sendCond = sync.NewCond(&sync.Mutex{})
	go s.readLoop()
	return s
}

func (s *H2Stream) Framer() *http2.Framer {
	return s.framer
}

func (s *H2Stream) LockWrite() {
	s.writeMut.Lock()
}

func (s *H2Stream) UnlockWrite() {
	s.writeMut.Unlock()
}

func (s *H2Stream) readLoop() {
	for {
		f, err := s.framer.ReadFrame()
		if err != nil {
			s.closeWithErr(err)
			return
		}

		switch f := f.(type) {
		case *http2.DataFrame:
			if f.StreamID == s.ActiveStream {
				s.readCond.L.Lock()
				s.readBuf.Write(f.Data())
				s.readCond.Broadcast()
				s.readCond.L.Unlock()
			}
		case *http2.HeadersFrame:
			if f.StreamEnded() && f.StreamID == s.ActiveStream {
				s.closeWithErr(io.EOF)
				return
			}
		case *http2.SettingsFrame:
			if !f.IsAck() {
				s.writeMut.Lock()
				s.framer.WriteSettingsAck()
				s.writeMut.Unlock()
			}
		case *http2.PingFrame:
			if !f.IsAck() {
				s.writeMut.Lock()
				s.framer.WritePing(true, f.Data)
				s.writeMut.Unlock()
			}
		case *http2.WindowUpdateFrame:
			s.sendCond.L.Lock()
			if f.StreamID == 0 {
				s.connWindow += int32(f.Increment)
			} else if f.StreamID == s.ActiveStream {
				s.streamWind += int32(f.Increment)
			}
			s.sendCond.Broadcast()
			s.sendCond.L.Unlock()
		case *http2.GoAwayFrame:
			s.closeWithErr(io.EOF)
			return
		}
	}
}

func (s *H2Stream) closeWithErr(err error) {
	s.readCond.L.Lock()
	s.closed = true
	s.readErr = err
	s.readCond.Broadcast()
	s.readCond.L.Unlock()

	s.sendCond.L.Lock()
	s.closed = true
	s.sendCond.Broadcast()
	s.sendCond.L.Unlock()

	s.conn.Close()
}

func (s *H2Stream) Read(b []byte) (n int, err error) {
	s.readCond.L.Lock()
	defer s.readCond.L.Unlock()

	for s.readBuf.Len() == 0 && !s.closed {
		s.readCond.Wait()
	}

	if s.readBuf.Len() > 0 {
		n, err = s.readBuf.Read(b)
		if n > 0 {
			// Send Window updates to keep the data flowing
			s.writeMut.Lock()
			_ = s.framer.WriteWindowUpdate(0, uint32(n))
			_ = s.framer.WriteWindowUpdate(s.ActiveStream, uint32(n))
			s.writeMut.Unlock()
		}
		return n, err
	}

	if s.readErr != nil {
		return 0, s.readErr
	}
	return 0, io.EOF
}

func (s *H2Stream) Write(b []byte) (n int, err error) {
	for len(b) > 0 {
		s.sendCond.L.Lock()
		for (s.connWindow <= 0 || s.streamWind <= 0) && !s.closed {
			s.sendCond.Wait()
		}
		if s.closed {
			s.sendCond.L.Unlock()
			return n, io.EOF
		}

		allowed := int32(len(b))
		if allowed > s.connWindow {
			allowed = s.connWindow
		}
		if allowed > s.streamWind {
			allowed = s.streamWind
		}
		if allowed > 16384 {
			allowed = 16384
		}

		s.connWindow -= allowed
		s.streamWind -= allowed
		s.sendCond.L.Unlock()

		chunk := b[:allowed]
		s.writeMut.Lock()
		err := s.framer.WriteData(s.ActiveStream, false, chunk)
		s.writeMut.Unlock()

		if err != nil {
			return n, err
		}

		b = b[allowed:]
		n += int(allowed)
	}
	return n, nil
}

func (s *H2Stream) Close() error {
	s.closeWithErr(io.EOF)
	return nil
}

func (s *H2Stream) LocalAddr() net.Addr                { return s.conn.LocalAddr() }
func (s *H2Stream) RemoteAddr() net.Addr               { return s.conn.RemoteAddr() }
func (s *H2Stream) SetDeadline(t time.Time) error      { return s.conn.SetDeadline(t) }
func (s *H2Stream) SetReadDeadline(t time.Time) error  { return s.conn.SetReadDeadline(t) }
func (s *H2Stream) SetWriteDeadline(t time.Time) error { return s.conn.SetWriteDeadline(t) }
