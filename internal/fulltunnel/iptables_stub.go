//go:build !linux

package fulltunnel

import (
	"fmt"

	"github.com/vakaka1/pp/internal/config"
)

func Up(cfg *config.ClientConfig, transparentListen string, owner string) error {
	_ = cfg
	_ = transparentListen
	_ = owner
	return fmt.Errorf("full-tunnel mode is supported only on Linux")
}

func Down() error {
	return fmt.Errorf("full-tunnel mode is supported only on Linux")
}
