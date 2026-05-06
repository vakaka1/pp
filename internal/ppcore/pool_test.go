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

func TestClientSmuxConfigClampsUnsafeKeepaliveInterval(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Transport.KeepaliveIntervalSeconds = 120

	smuxCfg := clientSmuxConfig(cfg)
	if smuxCfg.KeepAliveInterval >= smuxCfg.KeepAliveTimeout {
		t.Fatalf("keepalive interval must be below timeout, got interval=%s timeout=%s", smuxCfg.KeepAliveInterval, smuxCfg.KeepAliveTimeout)
	}
	if smuxCfg.KeepAliveInterval != protocol.DefaultSmuxConfig().KeepAliveTimeout/2 {
		t.Fatalf("unexpected clamped keepalive interval: %s", smuxCfg.KeepAliveInterval)
	}
}
