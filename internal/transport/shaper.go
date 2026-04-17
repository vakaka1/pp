package transport

import (
	"math/rand"
	"net"
	"time"
)

// Shaper implements traffic shaping with random jitter to defeat timing analysis.
type Shaper struct {
	net.Conn
	maxJitter time.Duration
}

// NewShaper creates a new traffic shaper wrapping a net.Conn.
func NewShaper(conn net.Conn, maxJitterMs int) *Shaper {
	return &Shaper{
		Conn:      conn,
		maxJitter: time.Duration(maxJitterMs) * time.Millisecond,
	}
}

func (s *Shaper) Read(p []byte) (n int, err error) {
	return s.Conn.Read(p)
}

func (s *Shaper) Write(p []byte) (n int, err error) {
	if s.maxJitter > 0 {
		jitter := time.Duration(rand.ExpFloat64() * float64(5*time.Millisecond))
		if jitter > s.maxJitter {
			jitter = s.maxJitter
		}
		time.Sleep(jitter)
	}
	return s.Conn.Write(p)
}
