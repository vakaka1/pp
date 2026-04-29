package ppcore

import (
	"context"
	"fmt"

	"github.com/vakaka1/pp/internal/config"
	"github.com/vakaka1/pp/internal/ppfallback"
	"go.uber.org/zap"
)

// Inbound represents a generic incoming connection handler.
type Inbound interface {
	Start(ctx context.Context) error
}

// Core is the orchestrator that manages multiple Inbounds.
type Core struct {
	cfg      *config.Config
	log      *zap.Logger
	inbounds []Inbound
}

// NewCore creates a new Core orchestrator.
func NewCore(cfg *config.Config, log *zap.Logger) (*Core, error) {
	core := &Core{
		cfg: cfg,
		log: log,
	}
	geoIP, geoSite := loadServerRoutingDatabases(log)

	for i, inbCfg := range cfg.Inbounds {
		switch inbCfg.Protocol {
		case "pp-fallback":
			settings, err := decodeFallbackSettings(inbCfg)
			if err != nil {
				return nil, fmt.Errorf("failed to decode inbound [%d] '%s' settings: %w", i, inbCfg.Tag, err)
			}

			engine, err := buildServerRoutingEngine(settings, geoIP, geoSite)
			if err != nil {
				return nil, fmt.Errorf("failed to build routing for inbound [%d] '%s': %w", i, inbCfg.Tag, err)
			}

			inbound, err := ppfallback.NewInbound(inbCfg, log, newInboundStreamHandler(engine))
			if err != nil {
				return nil, fmt.Errorf("failed to create pp-fallback inbound [%d] '%s': %w", i, inbCfg.Tag, err)
			}
			core.inbounds = append(core.inbounds, inbound)
		default:
			return nil, fmt.Errorf("unsupported protocol '%s' for inbound [%d] '%s'", inbCfg.Protocol, i, inbCfg.Tag)
		}
	}

	return core, nil
}

// Start launches all inbounds concurrently.
func (c *Core) Start(ctx context.Context) error {
	if len(c.inbounds) == 0 {
		return fmt.Errorf("no inbounds configured")
	}

	errChan := make(chan error, len(c.inbounds))

	for _, inb := range c.inbounds {
		go func(inb Inbound) {
			if err := inb.Start(ctx); err != nil {
				errChan <- err
			}
		}(inb)
	}

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}
