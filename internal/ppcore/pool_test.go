package ppcore

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/protocol"
	"github.com/xtaci/smux"
)

func TestOpenStreamStatusTimeoutDoesNotCloseSession(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	cfg := protocol.DefaultSmuxConfig()
	clientSession, err := smux.Client(clientConn, cfg)
	if err != nil {
		t.Fatalf("create client session: %v", err)
	}
	defer clientSession.Close()

	serverSession, err := smux.Server(serverConn, cfg)
	if err != nil {
		t.Fatalf("create server session: %v", err)
	}
	defer serverSession.Close()

	accepted := make(chan *smux.Stream, 1)
	go func() {
		stream, err := serverSession.AcceptStream()
		if err != nil {
			return
		}
		accepted <- stream
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err = openStreamOnSession(ctx, clientSession, "example.com:443")
	if err == nil {
		t.Fatalf("expected stream status timeout")
	}
	var netErr net.Error
	if !errors.Is(err, context.DeadlineExceeded) && (!errors.As(err, &netErr) || !netErr.Timeout()) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if clientSession.IsClosed() {
		t.Fatalf("stream status timeout closed the shared session")
	}

	select {
	case stream := <-accepted:
		_ = stream.Close()
	case <-time.After(time.Second):
		t.Fatalf("server did not receive the stream")
	}
}

func TestClientSmuxConfigUsesConfiguredKeepalive(t *testing.T) {
	disabled := clientSmuxConfig(&config.ClientConfig{})
	if !disabled.KeepAliveDisabled {
		t.Fatalf("smux keepalive must stay disabled without a configured interval")
	}

	cfg := &config.ClientConfig{}
	cfg.Transport.KeepaliveIntervalSeconds = 25

	smuxCfg := clientSmuxConfig(cfg)
	if smuxCfg.KeepAliveDisabled {
		t.Fatalf("smux keepalive must be enabled when configured")
	}
	if smuxCfg.KeepAliveInterval != 25*time.Second {
		t.Fatalf("unexpected keepalive interval: %v", smuxCfg.KeepAliveInterval)
	}
}

func TestClientSmuxConfigCapsKeepaliveInterval(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Transport.KeepaliveIntervalSeconds = 300

	smuxCfg := clientSmuxConfig(cfg)
	if smuxCfg.KeepAliveDisabled {
		t.Fatalf("smux keepalive must be enabled when configured")
	}
	if smuxCfg.KeepAliveInterval != smuxCfg.KeepAliveTimeout/2 {
		t.Fatalf("expected keepalive interval capped to half timeout, got %v", smuxCfg.KeepAliveInterval)
	}
}

func TestConnectionPoolStreamTimeoutThreshold(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientSession, err := smux.Client(clientConn, protocol.DefaultSmuxConfig())
	if err != nil {
		t.Fatalf("create client session: %v", err)
	}
	defer clientSession.Close()

	serverSession, err := smux.Server(serverConn, protocol.DefaultSmuxConfig())
	if err != nil {
		t.Fatalf("create server session: %v", err)
	}
	defer serverSession.Close()

	pool := NewConnectionPool(&config.ClientConfig{}, nil)
	pool.setSession(clientSession)

	for i := 1; i < maxConsecutiveStreamTimeouts; i++ {
		if pool.recordOpenStreamTimeout(clientSession) {
			t.Fatalf("timeout %d unexpectedly crossed reconnect threshold", i)
		}
	}
	if !pool.recordOpenStreamTimeout(clientSession) {
		t.Fatalf("expected timeout threshold to request reconnect")
	}

	pool.recordOpenStreamSuccess(clientSession)
	if pool.recordOpenStreamTimeout(clientSession) {
		t.Fatalf("success did not reset timeout threshold")
	}
}
