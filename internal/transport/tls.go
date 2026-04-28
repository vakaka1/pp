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
	return DialTLSWithALPN(targetAddr, serverName, profile, timeout, []string{"h2", "http/1.1"})
}

// DialTLSHTTP1 creates a browser-fingerprinted TLS connection pinned to HTTP/1.1.
func DialTLSHTTP1(targetAddr string, serverName string, profile string, timeout time.Duration) (net.Conn, error) {
	return DialTLSWithALPN(targetAddr, serverName, profile, timeout, []string{"http/1.1"})
}

// DialTLSWithALPN creates a uTLS connection that mimics the specified browser fingerprint
// and advertises the provided ALPN list.
func DialTLSWithALPN(targetAddr string, serverName string, profile string, timeout time.Duration, nextProtos []string) (net.Conn, error) {
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
		NextProtos:         nextProtos,
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
	}

	helloID := GetTLSProfile(profile)
	uConn := utls.UClient(rawConn, uTLSConfig, helloID)

	if len(nextProtos) > 0 {
		if err := uConn.BuildHandshakeState(); err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("utls build handshake failed: %w", err)
		}
		for _, ext := range uConn.Extensions {
			if a, ok := ext.(*utls.ALPNExtension); ok {
				a.AlpnProtocols = nextProtos
				break
			}
		}
	}

	if err := uConn.Handshake(); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("utls handshake failed: %w", err)
	}

	return uConn, nil
}
