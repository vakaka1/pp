package ppcore

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/user/pp/internal/config"
	"github.com/user/pp/internal/protocol"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

type ConnectionPool struct {
	cfg  *config.ClientConfig
	log  *zap.Logger
	mu   sync.Mutex
	sess *smux.Session
}

func NewConnectionPool(cfg *config.ClientConfig, log *zap.Logger) *ConnectionPool {
	return &ConnectionPool{
		cfg: cfg,
		log: log,
	}
}

func (p *ConnectionPool) Start(ctx context.Context) error {
	go p.maintainConnection(ctx)
	return nil
}

func (p *ConnectionPool) maintainConnection(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		noiseRunner := newBrowserNoiseRunner(p.cfg, p.log)
		sess, err := ConnectToServer(ctx, p.cfg, noiseRunner)
		if err != nil {
			p.log.Warn("failed to connect to server, retrying in 5s", zap.Error(err))
			time.Sleep(5 * time.Second)
			continue
		}

		p.mu.Lock()
		p.sess = sess
		p.mu.Unlock()
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
			p.log.Warn("session closed, reconnecting")
		case <-ctx.Done():
			presenceCancel()
			sess.Close()
			return
		}

		p.mu.Lock()
		p.sess = nil
		p.mu.Unlock()
	}
}

func (p *ConnectionPool) OpenStream(target string) (net.Conn, error) {
	p.mu.Lock()
	sess := p.sess
	p.mu.Unlock()

	if sess == nil || sess.IsClosed() {
		return nil, fmt.Errorf("no active session")
	}
	stream, err := sess.OpenStream()
	if err != nil {
		return nil, fmt.Errorf("open stream failed: %w", err)
	}

	host, portStr, _ := net.SplitHostPort(target)
	port, _ := strconv.Atoi(portStr)

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
		return nil, fmt.Errorf("failed to encode stream header: %w", err)
	}

	statusBuf := make([]byte, 1)
	if _, err := io.ReadFull(stream, statusBuf); err != nil {
		stream.Close()
		return nil, fmt.Errorf("failed to read stream status: %w", err)
	}

	if statusBuf[0] != protocol.StatusOK {
		stream.Close()
		return nil, fmt.Errorf("server rejected stream: status %x", statusBuf[0])
	}

	return stream, nil
}
