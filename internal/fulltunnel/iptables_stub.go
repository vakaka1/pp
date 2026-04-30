//go:build !linux && !windows

package fulltunnel

import (
	"fmt"

	"github.com/vakaka1/pp/internal/config"
)

func Up(cfg *config.ClientConfig, transparentListen string, owner string) error {
	_ = cfg
	_ = transparentListen
	_ = owner
	return fmt.Errorf("full-tunnel mode is not supported on this platform")
}

func Down() error {
	return fmt.Errorf("full-tunnel mode is not supported on this platform")
}
