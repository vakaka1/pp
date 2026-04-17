package transport

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	utls "github.com/refraction-networking/utls"
)

// DialTLS creates a uTLS connection that mimics the specified browser fingerprint.
func DialTLS(targetAddr string, serverName string, profile string, timeout time.Duration) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: timeout,
	}
	rawConn, err := dialer.Dial("tcp", targetAddr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial failed: %w", err)
	}

	uTLSConfig := &utls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: false, // ALWAYS false for security!
		NextProtos:         []string{"h2", "http/1.1"},
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
	}

	helloID := GetTLSProfile(profile)
	uConn := utls.UClient(rawConn, uTLSConfig, helloID)

	if err := uConn.Handshake(); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("utls handshake failed: %w", err)
	}

	return uConn, nil
}
