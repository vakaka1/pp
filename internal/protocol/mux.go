package protocol

import "github.com/xtaci/smux"

// DefaultSmuxConfig returns the default smux configuration.
func DefaultSmuxConfig() *smux.Config {
	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	cfg.MaxFrameSize = 16384
	cfg.MaxReceiveBuffer = 4194304
	cfg.MaxStreamBuffer = 65536
	return cfg
}
