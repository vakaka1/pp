package ppcore

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/crypto"
	"github.com/vakaka1/pp/internal/protocol"
	"github.com/vakaka1/pp/internal/transport"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
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

	framer := http2.NewFramer(conn, conn)

	// Set a deadline for the handshake process
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	if err := framer.WriteSettings(protocol.GetChromeSettings()...); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write settings failed: %w", err)
	}
	if err := framer.WriteWindowUpdate(0, 15663105); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write window update failed: %w", err)
	}

	streamID := uint32(1)
	if noise != nil {
		browseCtx, cancel := context.WithTimeout(ctx, browserNoisePreconnectTimeout)
		streamID, err = runBrowserWarmupOnH2(browseCtx, framer, cfg, noise, streamID)
		cancel()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("browser warmup failed: %w", err)
		}
	}

	h2 := protocol.NewH2StreamWithFramer(conn, framer, streamID)
	h2.LockWrite()

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

	smuxCfg := clientSmuxConfig(cfg)
	session, err := smux.Client(transportConn, smuxCfg)
	if err != nil {
		transportConn.Close()
		return nil, fmt.Errorf("smux client failed: %w", err)
	}

	// Reset deadline for the established session
	conn.SetDeadline(time.Time{})

	return session, nil
}

func runBrowserWarmupOnH2(ctx context.Context, framer *http2.Framer, cfg *config.ClientConfig, noise *browserNoiseRunner, streamID uint32) (uint32, error) {
	if _, err := fetchWarmupPage(ctx, framer, cfg, noise, streamID, "/", ""); err != nil {
		noise.log.Debug("browser warmup landing page failed", zap.Error(err))
		return streamID, err
	}
	streamID += 2

	if !noise.pause(ctx, noise.randomDuration(browserNoiseThinkMin, browserNoiseThinkMax)) {
		return streamID, ctx.Err()
	}

	if _, err := fetchWarmupPage(ctx, framer, cfg, noise, streamID, protocol.LoginTunnelPath, "https://"+cfg.Server.Domain+"/"); err != nil {
		noise.log.Debug("browser warmup login page failed", zap.Error(err))
		return streamID, err
	}
	streamID += 2

	return streamID, nil
}

func fetchWarmupPage(ctx context.Context, framer *http2.Framer, cfg *config.ClientConfig, noise *browserNoiseRunner, streamID uint32, path, referer string) (browserNoisePage, error) {
	headers := []hpack.HeaderField{
		{Name: ":method", Value: http.MethodGet},
		{Name: ":scheme", Value: "https"},
		{Name: ":path", Value: path},
		{Name: ":authority", Value: cfg.Server.Domain},
		{Name: "user-agent", Value: noise.userAgent},
		{Name: "accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
		{Name: "accept-language", Value: "ru-RU,ru;q=0.9,en-US;q=0.7,en;q=0.5"},
		{Name: "cache-control", Value: "max-age=0"},
		{Name: "upgrade-insecure-requests", Value: "1"},
	}
	if referer != "" {
		headers = append(headers, hpack.HeaderField{Name: "referer", Value: referer})
	}

	if err := protocol.WriteHeaders(framer, streamID, true, headers); err != nil {
		return browserNoisePage{}, err
	}

	body, err := readWarmupResponse(ctx, framer, streamID)
	if err != nil {
		return browserNoisePage{}, err
	}
	return browserNoisePage{articlePaths: extractBrowserNoiseArticlePaths(string(body))}, nil
}

func readWarmupResponse(ctx context.Context, framer *http2.Framer, streamID uint32) ([]byte, error) {
	var body []byte
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		frame, err := framer.ReadFrame()
		if err != nil {
			return nil, err
		}

		switch f := frame.(type) {
		case *http2.DataFrame:
			if f.StreamID != streamID {
				continue
			}
			if len(body) < 2<<20 {
				remaining := (2 << 20) - len(body)
				data := f.Data()
				if len(data) > remaining {
					data = data[:remaining]
				}
				body = append(body, data...)
			}
			if f.StreamEnded() {
				return body, nil
			}
		case *http2.HeadersFrame:
			if f.StreamID == streamID && f.StreamEnded() {
				return body, nil
			}
		case *http2.SettingsFrame:
			if !f.IsAck() {
				if err := framer.WriteSettingsAck(); err != nil {
					return nil, err
				}
			}
		case *http2.PingFrame:
			if !f.IsAck() {
				if err := framer.WritePing(true, f.Data); err != nil {
					return nil, err
				}
			}
		case *http2.RSTStreamFrame:
			if f.StreamID == streamID {
				return nil, fmt.Errorf("warmup stream reset: %v", f.ErrCode)
			}
		case *http2.GoAwayFrame:
			return nil, io.EOF
		}
	}
}

func clientSmuxConfig(cfg *config.ClientConfig) *smux.Config {
	smuxCfg := protocol.DefaultSmuxConfig()
	if cfg == nil || cfg.Transport.KeepaliveIntervalSeconds <= 0 {
		return smuxCfg
	}

	keepAliveInterval := time.Duration(cfg.Transport.KeepaliveIntervalSeconds) * time.Second
	if keepAliveInterval >= smuxCfg.KeepAliveTimeout {
		keepAliveInterval = smuxCfg.KeepAliveTimeout / 2
	}
	smuxCfg.KeepAliveInterval = keepAliveInterval
	return smuxCfg
}
