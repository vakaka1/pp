package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// HandleSOCKS5 processes a SOCKS5 client connection.
// It returns the target address requested by the client.
func HandleSOCKS5(conn net.Conn) (*Request, error) {
	// 1. Handshake
	buf := make([]byte, 256)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return nil, err
	}
	if buf[0] != 0x05 {
		return nil, fmt.Errorf("invalid socks version")
	}
	numMethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:numMethods]); err != nil {
		return nil, err
	}
	// Reply: NO AUTH
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return nil, err
	}

	// 2. Request
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return nil, err
	}
	if buf[0] != 0x05 || buf[1] != 0x01 || buf[2] != 0x00 {
		return nil, fmt.Errorf("invalid request")
	}

	addrType := buf[3]
	var address string
	switch addrType {
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return nil, err
		}
		address = net.IP(buf[:4]).String()
	case 0x03: // Domain
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return nil, err
		}
		domainLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:domainLen]); err != nil {
			return nil, err
		}
		address = string(buf[:domainLen])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return nil, err
		}
		address = net.IP(buf[:16]).String()
	default:
		return nil, fmt.Errorf("unsupported addr type: %d", addrType)
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return nil, err
	}
	port := binary.BigEndian.Uint16(buf[:2])

	// Reply success
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return nil, err
	}

	return &Request{
		Target: fmt.Sprintf("%s:%d", address, port),
	}, nil
}
