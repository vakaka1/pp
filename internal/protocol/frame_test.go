package protocol

import (
	"bytes"
	"testing"
)

func TestPPStreamHeader_IPv4(t *testing.T) {
	h := &PPStreamHeader{
		AddrType: AddrTypeIPv4,
		Address:  "192.168.1.1",
		Port:     443,
	}
	buf := new(bytes.Buffer)
	if err := h.Encode(buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	h2 := &PPStreamHeader{}
	if err := h2.Decode(buf); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if h2.Address != h.Address || h2.Port != h.Port || h2.AddrType != h.AddrType {
		t.Fatalf("Mismatch: %+v != %+v", h, h2)
	}
}

func TestPPStreamHeader_IPv6(t *testing.T) {
	h := &PPStreamHeader{
		AddrType: AddrTypeIPv6,
		Address:  "2001:db8::1",
		Port:     8080,
	}
	buf := new(bytes.Buffer)
	if err := h.Encode(buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	h2 := &PPStreamHeader{}
	if err := h2.Decode(buf); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if h2.Address != h.Address || h2.Port != h.Port || h2.AddrType != h.AddrType {
		t.Fatalf("Mismatch: %+v != %+v", h, h2)
	}
}

func TestPPStreamHeader_Domain(t *testing.T) {
	domain := "example.com"
	h := &PPStreamHeader{
		AddrType: AddrTypeDomain,
		AddrLen:  uint8(len(domain)),
		Address:  domain,
		Port:     80,
	}
	buf := new(bytes.Buffer)
	if err := h.Encode(buf); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	h2 := &PPStreamHeader{}
	if err := h2.Decode(buf); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if h2.Address != h.Address || h2.Port != h.Port || h2.AddrType != h.AddrType || h2.AddrLen != h.AddrLen {
		t.Fatalf("Mismatch: %+v != %+v", h, h2)
	}
}
