package ppfallback

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/flynn/noise"
	"github.com/user/pp/internal/antireplay"
	"github.com/user/pp/internal/config"
	"github.com/user/pp/internal/crypto"
	"github.com/user/pp/internal/protocol"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type Inbound struct {
	tag             string
	listen          string
	tls             *config.TLSConfig
	settings        config.FallbackSettings
	log             *zap.Logger
	jtiCache        *antireplay.JTICache
	// psks holds one or more decoded pre-shared keys accepted for JWT validation.
	// Per-client PSKs are supported by populating this list with each client's PSK.
	psks            [][]byte
	noiseCfg        *protocol.NoiseConfig
	db              *FallbackDB
	contentLoader   *ContentLoader
	fallbackHandler *FallbackHandler
	httpServer      *http.Server
	streamHandler   func(*smux.Stream, *zap.Logger)
}

func NewInbound(inb config.InboundConfig, log *zap.Logger, streamHandler func(*smux.Stream, *zap.Logger)) (*Inbound, error) {
	if streamHandler == nil {
		return nil, fmt.Errorf("stream handler is required")
	}

	var settings config.FallbackSettings
	if err := json.Unmarshal(inb.Settings, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse fallback settings: %w", err)
	}

	// Build the list of accepted PSKs.
	// If a per-client psks list is provided, use those. Otherwise fall back to single psk.
	var psks [][]byte
	if len(settings.PSKs) > 0 {
		for _, pskStr := range settings.PSKs {
			pskBytes, err := crypto.DecodeKey(pskStr)
			if err != nil {
				return nil, fmt.Errorf("invalid key in psks list: %w", err)
			}
			psks = append(psks, pskBytes)
		}
	} else {
		pskBytes, err := crypto.DecodeKey(settings.PSK)
		if err != nil {
			return nil, err
		}
		psks = [][]byte{pskBytes}
	}

	privBytes, err := crypto.DecodeKey(settings.NoisePrivateKey)
	if err != nil {
		return nil, err
	}
	pubBase64, err := crypto.DerivePublicKey(settings.NoisePrivateKey)
	if err != nil {
		return nil, err
	}
	pubBytes, err := crypto.DecodeKey(pubBase64)
	if err != nil {
		return nil, err
	}

	resolvedDBPath := ResolveFallbackDBPath(settings.DBPath, inb.Tag)
	db, err := InitFallbackDB(resolvedDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init fallback db: %w", err)
	}
	settings.DBPath = resolvedDBPath

	fallbackType := strings.TrimSpace(settings.Type)
	if fallbackType == "" {
		fallbackType = "blog"
	}

	inboundLog := log.With(zap.String("inbound", inb.Tag))
	if strings.TrimSpace(inb.Tag) != "" {
		inboundLog.Info("resolved fallback content storage", zap.String("path", resolvedDBPath))
	}

	var contentLoader *ContentLoader
	if fallbackType != "proxy" {
		contentLoader = NewContentLoader(db, settings.ScraperKeywords, settings.PublishIntervalMinutes, settings.PublishBatchSize, inboundLog)
	}

	fallbackHandler, err := NewFallbackHandler(settings.Type, settings.ProxyAddress, settings.InviteCode, db)
	if err != nil {
		return nil, err
	}

	noiseCfg := &protocol.NoiseConfig{
		StaticKeypair: noise.DHKey{
			Private: privBytes,
			Public:  pubBytes,
		},
		IsClient:     false,
		ServerDomain: settings.Domain,
	}

	capacity := settings.AntiReplay.BloomCapacity
	if capacity == 0 {
		capacity = 100000
	}
	errRate := settings.AntiReplay.BloomErrorRate
	if errRate == 0 {
		errRate = 0.001
	}
	rotMins := settings.AntiReplay.RotationMinutes
	if rotMins == 0 {
		rotMins = 8
	}

	return &Inbound{
		tag:             inb.Tag,
		listen:          inb.Listen,
		tls:             inb.TLS,
		settings:        settings,
		log:             inboundLog,
		jtiCache:        antireplay.NewJTICache(capacity, errRate, time.Duration(rotMins)*time.Minute),
		psks:            psks,
		noiseCfg:        noiseCfg,
		db:              db,
		contentLoader:   contentLoader,
		fallbackHandler: fallbackHandler,
		streamHandler:   streamHandler,
	}, nil
}

func (s *Inbound) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.settings.GRPCPath, s.handleGRPC)
	mux.HandleFunc("/", s.handleFallback)

	h2s := &http2.Server{}
	h1s := &http.Server{
		Addr:    s.listen,
		Handler: h2c.NewHandler(mux, h2s),
	}
	s.httpServer = h1s

	if s.contentLoader != nil {
		go s.contentLoader.Run(ctx)
	}

	go func() {
		<-ctx.Done()
		s.httpServer.Close()
	}()

	if s.tls != nil && s.tls.Enabled {
		s.log.Info("fallback inbound listening with TLS", zap.String("address", s.listen), zap.String("cert", s.tls.CertFile))
		err := s.httpServer.ListenAndServeTLS(s.tls.CertFile, s.tls.KeyFile)
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	} else {
		s.log.Info("fallback inbound listening (h2c)", zap.String("address", s.listen))
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	}
	return nil
}

func (s *Inbound) handleFallback(w http.ResponseWriter, r *http.Request) {
	s.fallbackHandler.ServeHTTP(w, r)
}

func (s *Inbound) handleGRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		s.handleFallback(w, r)
		return
	}

	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		s.handleFallback(w, r)
		return
	}
	token := auth[7:]
	// Try each accepted PSK; the first successful validation wins.
	var jwtValid bool
	for _, psk := range s.psks {
		valid, err := protocol.ValidateJWT(token, psk, 15*time.Minute, s.jtiCache.CheckAndAdd)
		if valid {
			jwtValid = true
			break
		}
		_ = err
	}
	if !jwtValid {
		s.log.Debug("invalid jwt - no matching psk")
		s.handleFallback(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/grpc")
	w.Header().Set("Grpc-Encoding", "identity")
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	conn := &protocol.HttpConn{R: r.Body, W: w}
	sendCipher, recvCipher, err := protocol.PerformNoiseNKHandshake(conn, s.noiseCfg)
	if err != nil {
		s.log.Debug("noise handshake failed", zap.Error(err))
		return
	}

	noiseConn := protocol.NewNoiseConn(conn, sendCipher, recvCipher)
	session, err := smux.Server(noiseConn, protocol.DefaultSmuxConfig())
	if err != nil {
		s.log.Debug("smux session failed", zap.Error(err))
		return
	}

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			break
		}
		if s.contentLoader != nil {
			s.contentLoader.MarkProxyActivity()
		}
		go s.streamHandler(stream, s.log)
	}
}
