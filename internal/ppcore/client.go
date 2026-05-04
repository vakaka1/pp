package ppcore

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/proxy"
	"github.com/vakaka1/pp/internal/routing"
	"go.uber.org/zap"
)

// Client is the main core client structure.
type Client struct {
	cfg    *config.ClientConfig
	log    *zap.Logger
	engine *routing.Engine
	pool   *ConnectionPool
}

// NewClient creates a new PP client.
func NewClient(cfg *config.ClientConfig, log *zap.Logger, engine *routing.Engine) *Client {
	return &Client{
		cfg:    cfg,
		log:    log,
		engine: engine,
		pool:   NewConnectionPool(cfg, log),
	}
}

// Start starts the client proxy listeners.
func (c *Client) Start(ctx context.Context) error {
	if err := c.pool.Start(ctx); err != nil {
		return err
	}

	if c.cfg.Socks5Listen != "" {
		l, err := net.Listen("tcp", c.cfg.Socks5Listen)
		if err != nil {
			return err
		}
		c.log.Info("SOCKS5 server started", zap.String("address", c.cfg.Socks5Listen))
		go c.acceptLoop(ctx, l, proxy.HandleSOCKS5)
	}

	if c.cfg.HTTPProxyListen != "" {
		l, err := net.Listen("tcp", c.cfg.HTTPProxyListen)
		if err != nil {
			return err
		}
		c.log.Info("HTTP proxy server started", zap.String("address", c.cfg.HTTPProxyListen))
		go c.acceptLoop(ctx, l, proxy.HandleHTTP)
	}

	if c.cfg.TransparentListen != "" {
		l, err := net.Listen("tcp", c.cfg.TransparentListen)
		if err != nil {
			return err
		}
		c.log.Info("transparent proxy server started", zap.String("address", c.cfg.TransparentListen))
		go c.acceptLoop(ctx, l, proxy.HandleTransparent)
	}

	return nil
}

func (c *Client) acceptLoop(ctx context.Context, l net.Listener, handler func(net.Conn) (*proxy.Request, error)) {
	for {
		select {
		case <-ctx.Done():
			l.Close()
			return
		default:
		}
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go c.handleClientConn(conn, handler)
	}
}

func (c *Client) handleClientConn(conn net.Conn, handler func(net.Conn) (*proxy.Request, error)) {
	defer conn.Close()
	request, err := handler(conn)
	if err != nil {
		c.log.Debug("proxy handler failed", zap.Error(err))
		return
	}
	target := request.Target

	host, _, _ := net.SplitHostPort(target)
	ip := net.ParseIP(host)

	policy := c.engine.Route(host, ip)
	if policy == routing.PolicyBlock {
		return
	}

	var remote net.Conn
	if policy == routing.PolicyDirect {
		remote, err = net.DialTimeout("tcp", target, 5*time.Second)
		if err != nil {
			return
		}
	} else {
		// Proxy
		remote, err = c.pool.OpenStream(target)
		if err != nil {
			var rejected *StreamRejectedError
			if errors.As(err, &rejected) {
				c.log.Debug("server rejected stream", zap.String("target", target), zap.Uint8("status", rejected.Status))
			} else if errors.Is(err, errSessionClosed) {
				c.log.Debug("session is closed, dropping proxied request", zap.String("target", target))
			} else {
				c.log.Warn("failed to open stream", zap.String("target", target), zap.Error(err))
			}
			return
		}
	}
	defer remote.Close()

	if len(request.InitialData) > 0 {
		if _, err := remote.Write(request.InitialData); err != nil {
			c.log.Debug("failed to send initial upstream payload", zap.Error(err))
			return
		}
	}

	// Relay
	errc := make(chan error, 2)
	go func() {
		_, err := proxy.Copy(remote, conn)
		errc <- err
	}()
	go func() {
		_, err := proxy.Copy(conn, remote)
		errc <- err
	}()
	<-errc
}
