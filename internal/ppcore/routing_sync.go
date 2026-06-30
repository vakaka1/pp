package ppcore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/crypto"
	"github.com/vakaka1/pp/internal/protocol"
	"github.com/vakaka1/pp/internal/routing"
	"github.com/vakaka1/pp/internal/transport"
	"go.uber.org/zap"
)

const routingSyncInterval = 30 * time.Second

func (c *Client) syncRemoteRouting(ctx context.Context) {
	ticker := time.NewTicker(routingSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.refreshRemoteRouting(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) refreshRemoteRouting(ctx context.Context) {
	routingCfg, err := c.fetchRemoteRouting(ctx)
	if err != nil {
		c.log.Debug("failed to sync routing from server", zap.Error(err))
		return
	}

	engine, err := routing.NewEngine(*routingCfg, c.geoIP, c.geoSite)
	if err != nil {
		c.log.Warn("server routing config is invalid", zap.Error(err))
		return
	}

	c.setRoutingEngine(engine)
	c.log.Info("routing synced from server", zap.Int("rules", len(routingCfg.Rules)), zap.String("default_policy", routingCfg.DefaultPolicy))
}

func (c *Client) fetchRemoteRouting(ctx context.Context) (*config.RoutingConfig, error) {
	if c.cfg == nil {
		return nil, fmt.Errorf("client config is required")
	}

	psk, err := crypto.DecodeKey(c.cfg.Server.PSK)
	if err != nil {
		return nil, fmt.Errorf("invalid psk: %w", err)
	}
	token, err := protocol.GenerateJWT(psk, randomHex(16), uuidV4(), time.Now(), time.Now().Add(10*time.Minute))
	if err != nil {
		return nil, fmt.Errorf("jwt generation failed: %w", err)
	}

	url := "https://" + c.cfg.Server.Domain + protocol.RoutingSyncPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.Server.GRPCUserAgent)

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2: false,
			DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
				return transport.DialTLSHTTP1(c.cfg.Server.Address, c.cfg.Server.Domain, c.cfg.Server.TLSFingerprint, 10*time.Second)
			},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("routing sync failed: %s: %s", resp.Status, string(body))
	}

	var serverCfg config.ServerRoutingConfig
	if err := json.NewDecoder(resp.Body).Decode(&serverCfg); err != nil {
		return nil, err
	}

	return &config.RoutingConfig{
		DefaultPolicy: serverCfg.DefaultPolicy,
		Rules:         serverCfg.Rules,
	}, nil
}
