package protocol

import (
	"time"

	"github.com/xtaci/smux"
)

// DefaultSmuxConfig returns the default smux configuration.
func DefaultSmuxConfig() *smux.Config {
	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	cfg.MaxFrameSize = 16384
	cfg.MaxReceiveBuffer = 4194304
	cfg.MaxStreamBuffer = 65536
	return cfg
}

// ServerSmuxConfig keeps long-lived tunnel sessions active even when no user
// streams are open. This is important behind NATs and HTTP reverse proxies with
// idle connection timeouts.
func ServerSmuxConfig() *smux.Config {
	cfg := DefaultSmuxConfig()
	cfg.KeepAliveDisabled = false
	cfg.KeepAliveInterval = 25 * time.Second
	if cfg.KeepAliveInterval >= cfg.KeepAliveTimeout {
		cfg.KeepAliveInterval = cfg.KeepAliveTimeout / 2
	}
	return cfg
}
