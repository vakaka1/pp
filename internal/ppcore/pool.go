package ppcore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/protocol"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

type ConnectionPool struct {
	cfg   *config.ClientConfig
	log   *zap.Logger
	mu    sync.Mutex
	sess  *smux.Session
	ready chan struct{}
	done  bool
}

func NewConnectionPool(cfg *config.ClientConfig, log *zap.Logger) *ConnectionPool {
	return &ConnectionPool{
		cfg:   cfg,
		log:   log,
		ready: make(chan struct{}),
	}
}

func (p *ConnectionPool) Start(ctx context.Context) error {
	go p.maintainConnection(ctx)
	return nil
}

func (p *ConnectionPool) maintainConnection(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	noiseRunner := newBrowserNoiseRunner(p.cfg, p.log)
	sess, err := ConnectToServer(ctx, p.cfg, noiseRunner)
	if err != nil {
		p.log.Warn("failed to connect to server", zap.Error(err))
		p.closePool()
		return
	}

	p.setSession(sess)
	p.log.Info("connected to server successfully")

	presenceCtx, presenceCancel := context.WithCancel(ctx)
	go noiseRunner.RunPresenceLoop(presenceCtx)

	closedCh := make(chan struct{})
	go func() {
		_, _ = sess.AcceptStream()
		close(closedCh)
	}()

	select {
	case <-closedCh:
		presenceCancel()
		p.log.Warn("session closed")
	case <-ctx.Done():
		presenceCancel()
		sess.Close()
		return
	}

	p.clearSession(sess)
	p.closePool()
}

var errSessionUnavailable = errors.New("session unavailable")
var errSessionClosed = errors.New("session closed")

type StreamRejectedError struct {
	Status byte
}

func (e *StreamRejectedError) Error() string {
	return fmt.Sprintf("server rejected stream: status %x", e.Status)
}

func (p *ConnectionPool) OpenStream(target string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.OpenStreamContext(ctx, target)
}

func (p *ConnectionPool) OpenStreamContext(ctx context.Context, target string) (net.Conn, error) {
	for {
		sess, ready, done := p.currentSession()
		if done {
			return nil, errSessionClosed
		}
		if sess == nil {
			if err := waitForSession(ctx, ready); err != nil {
				return nil, err
			}
			continue
		}
		if sess.IsClosed() {
			p.clearSession(sess)
			continue
		}

		stream, err := openStreamOnSession(sess, target)
		if err == nil {
			return stream, nil
		}
		if !errors.Is(err, errSessionUnavailable) {
			return nil, err
		}

		p.clearSession(sess)
	}
}

func (p *ConnectionPool) currentSession() (*smux.Session, <-chan struct{}, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sess := p.sess
	ready := p.ready
	done := p.done
	return sess, ready, done
}

func (p *ConnectionPool) setSession(sess *smux.Session) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		_ = sess.Close()
		return
	}
	p.sess = sess
	close(p.ready)
}

func (p *ConnectionPool) clearSession(sess *smux.Session) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sess != sess {
		return
	}
	p.sess = nil
	p.ready = make(chan struct{})
	_ = sess.Close()
}

func (p *ConnectionPool) closePool() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return
	}
	p.done = true
	close(p.ready)
}

func waitForSession(ctx context.Context, ready <-chan struct{}) error {
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", errSessionUnavailable, ctx.Err())
	}
}

func openStreamOnSession(sess *smux.Session, target string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return nil, fmt.Errorf("invalid target %q: %w", target, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid target port %q", portStr)
	}

	stream, err := sess.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("%w: open stream failed: %w", errSessionUnavailable, err)
	}

	var hdr protocol.PPStreamHeader
	hdr.Port = uint16(port)
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() != nil {
			hdr.AddrType = protocol.AddrTypeIPv4
		} else {
			hdr.AddrType = protocol.AddrTypeIPv6
		}
		hdr.Address = ip.String()
	} else {
		hdr.AddrType = protocol.AddrTypeDomain
		hdr.Address = host
		hdr.AddrLen = uint8(len(host))
	}

	if err := hdr.Encode(stream); err != nil {
		stream.Close()
		return nil, fmt.Errorf("%w: failed to encode stream header: %w", errSessionUnavailable, err)
	}

	statusBuf := make([]byte, 1)
	if _, err := io.ReadFull(stream, statusBuf); err != nil {
		stream.Close()
		return nil, fmt.Errorf("%w: failed to read stream status: %w", errSessionUnavailable, err)
	}

	if statusBuf[0] != protocol.StatusOK {
		stream.Close()
		return nil, &StreamRejectedError{Status: statusBuf[0]}
	}

	return stream, nil
}
