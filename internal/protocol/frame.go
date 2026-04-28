package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

const (
	AddrTypeIPv4   = 0x01
	AddrTypeDomain = 0x03
	AddrTypeIPv6   = 0x04
)

const (
	StatusOK          = 0x00
	StatusUnreachable = 0x01
	StatusConnRefused = 0x02
	StatusNetUnreach  = 0x03
	StatusInternalErr = 0xFF
)

// PPStreamHeader represents the initial request sent on an smux stream.
type PPStreamHeader struct {
	AddrType uint8
	AddrLen  uint8 // Only used for AddrTypeDomain
	Address  string
	Port     uint16
}

// Encode writes the PPStreamHeader to the writer.
func (h *PPStreamHeader) Encode(w io.Writer) error {
	var buf []byte
	buf = append(buf, h.AddrType)

	switch h.AddrType {
	case AddrTypeIPv4:
		ip := net.ParseIP(h.Address).To4()
		if ip == nil {
			return fmt.Errorf("invalid ipv4 address")
		}
		buf = append(buf, ip...)
	case AddrTypeIPv6:
		ip := net.ParseIP(h.Address).To16()
		if ip == nil {
			return fmt.Errorf("invalid ipv6 address")
		}
		buf = append(buf, ip...)
	case AddrTypeDomain:
		buf = append(buf, h.AddrLen)
		buf = append(buf, []byte(h.Address)...)
	default:
		return fmt.Errorf("unsupported addr type: %x", h.AddrType)
	}

	portBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(portBuf, h.Port)
	buf = append(buf, portBuf...)

	if _, err := w.Write(buf); err != nil {
		return err
	}
	return nil
}

// Decode reads the PPStreamHeader from the reader.
func (h *PPStreamHeader) Decode(r io.Reader) error {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	h.AddrType = buf[0]

	switch h.AddrType {
	case AddrTypeIPv4:
		ipBuf := make([]byte, 4)
		if _, err := io.ReadFull(r, ipBuf); err != nil {
			return err
		}
		h.Address = net.IP(ipBuf).String()
	case AddrTypeIPv6:
		ipBuf := make([]byte, 16)
		if _, err := io.ReadFull(r, ipBuf); err != nil {
			return err
		}
		h.Address = net.IP(ipBuf).String()
	case AddrTypeDomain:
		if _, err := io.ReadFull(r, buf); err != nil {
			return err
		}
		h.AddrLen = buf[0]
		domainBuf := make([]byte, h.AddrLen)
		if _, err := io.ReadFull(r, domainBuf); err != nil {
			return err
		}
		h.Address = string(domainBuf)
	default:
		return fmt.Errorf("unsupported addr type: %x", h.AddrType)
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, portBuf); err != nil {
		return err
	}
	h.Port = binary.BigEndian.Uint16(portBuf)

	return nil
}

// AddressString returns the host:port format string.
func (h *PPStreamHeader) AddressString() string {
	return net.JoinHostPort(h.Address, strconv.Itoa(int(h.Port)))
}
