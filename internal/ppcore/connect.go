package ppcore

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/crypto"
	"github.com/vakaka1/pp/internal/protocol"
	"github.com/vakaka1/pp/internal/transport"
	"github.com/xtaci/smux"
	"golang.org/x/net/http2"
)

func uuidV4() string {
	b := make([]byte, 16)
	crand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	crand.Read(b)
	return hex.EncodeToString(b)
}

func ConnectToServer(ctx context.Context, cfg *config.ClientConfig, noise *browserNoiseRunner) (*smux.Session, error) {
	if noise != nil {
		browseCtx, cancel := context.WithTimeout(ctx, browserNoisePreconnectTimeout)
		noise.runPreConnectScenario(browseCtx)
		cancel()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	conn, err := transport.DialTLS(cfg.Server.Address, cfg.Server.Domain, cfg.Server.TLSFingerprint, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("tls dial failed: %w", err)
	}

	if _, err := conn.Write([]byte(http2.ClientPreface)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write preface failed: %w", err)
	}

	h2 := protocol.NewH2Stream(conn)
	h2.LockWrite()

	// Set a deadline for the handshake process
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	if err := h2.Framer().WriteSettings(protocol.GetChromeSettings()...); err != nil {
		h2.UnlockWrite()
		h2.Close()
		return nil, fmt.Errorf("write settings failed: %w", err)
	}
	if err := h2.Framer().WriteWindowUpdate(0, 15663105); err != nil {
		h2.UnlockWrite()
		h2.Close()
		return nil, fmt.Errorf("write window update failed: %w", err)
	}

	streamID := uint32(1)
	h2.ActiveStream = streamID

	if err := h2.Framer().WriteWindowUpdate(streamID, 15663105); err != nil {
		h2.UnlockWrite()
		h2.Close()
		return nil, fmt.Errorf("write stream window update failed: %w", err)
	}

	psk, _ := crypto.DecodeKey(cfg.Server.PSK)
	jti := randomHex(16)
	sub := uuidV4()
	jwtToken, err := protocol.GenerateJWT(psk, jti, sub, time.Now(), time.Now().Add(10*time.Minute))
	if err != nil {
		h2.UnlockWrite()
		h2.Close()
		return nil, fmt.Errorf("jwt generation failed: %w", err)
	}

	// The authenticated gRPC request is the login action. A normal GET /login
	// renders the fallback page; a POST /login with a valid token opens the tunnel.
	headers := protocol.GenerateGRPCClientHeaders(cfg.Server.Domain, protocol.LoginTunnelPath, jwtToken, cfg.Server.GRPCUserAgent)
	if err := protocol.WriteHeaders(h2.Framer(), streamID, false, headers); err != nil {
		h2.UnlockWrite()
		h2.Close()
		return nil, fmt.Errorf("write headers failed: %w", err)
	}
	h2.UnlockWrite()

	serverPub, _ := crypto.DecodeKey(cfg.Server.NoisePublicKey)
	noiseCfg := &protocol.NoiseConfig{
		ServerPublic: serverPub,
		IsClient:     true,
		ServerDomain: cfg.Server.Domain,
	}

	sendCipher, recvCipher, err := protocol.PerformNoiseNKHandshake(h2, noiseCfg)
	if err != nil {
		h2.Close()
		return nil, fmt.Errorf("noise handshake failed: %w", err)
	}

	noiseConn := protocol.NewNoiseConn(h2, sendCipher, recvCipher)

	var transportConn net.Conn = noiseConn
	if cfg.Transport.ShaperEnabled {
		transportConn = transport.NewShaper(noiseConn, cfg.Transport.JitterMaxMs)
	}

	smuxCfg := protocol.DefaultSmuxConfig()
	if cfg.Transport.KeepaliveIntervalSeconds > 0 {
		smuxCfg.KeepAliveInterval = time.Duration(cfg.Transport.KeepaliveIntervalSeconds) * time.Second
	}
	session, err := smux.Client(transportConn, smuxCfg)
	if err != nil {
		transportConn.Close()
		return nil, fmt.Errorf("smux client failed: %w", err)
	}

	// Reset deadline for the established session
	conn.SetDeadline(time.Time{})

	return session, nil
}
