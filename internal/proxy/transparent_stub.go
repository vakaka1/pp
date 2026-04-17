//go:build !linux

package proxy

import (
	"fmt"
	"net"
)

// HandleTransparent is unavailable on non-Linux platforms.
func HandleTransparent(conn net.Conn) (*Request, error) {
	_ = conn
	return nil, fmt.Errorf("transparent redirect mode is supported only on Linux")
}
