package crypto

import (
	"encoding/base64"
	"testing"
)

func TestGenerateX25519KeyPair(t *testing.T) {
	priv, pub, err := GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateX25519KeyPair failed: %v", err)
	}
	if len(priv) == 0 || len(pub) == 0 {
		t.Fatalf("Empty keys generated")
	}

	privBytes, err := DecodeKey(priv)
	if err != nil {
		t.Fatalf("Failed to decode private key: %v", err)
	}
	if len(privBytes) != 32 {
		t.Fatalf("Private key length is %d, expected 32", len(privBytes))
	}

	pubBytes, err := DecodeKey(pub)
	if err != nil {
		t.Fatalf("Failed to decode public key: %v", err)
	}
	if len(pubBytes) != 32 {
		t.Fatalf("Public key length is %d, expected 32", len(pubBytes))
	}
}

func TestGeneratePSK(t *testing.T) {
	psk, err := GeneratePSK()
	if err != nil {
		t.Fatalf("GeneratePSK failed: %v", err)
	}
	pskBytes, err := DecodeKey(psk)
	if err != nil {
		t.Fatalf("Failed to decode PSK: %v", err)
	}
	if len(pskBytes) != 32 {
		t.Fatalf("PSK length is %d, expected 32", len(pskBytes))
	}
}

func TestDecodeKeyInvalid(t *testing.T) {
	_, err := DecodeKey("invalid base64!")
	if err == nil {
		t.Fatalf("Expected error for invalid base64")
	}

	shortKey := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	_, err = DecodeKey(shortKey)
	if err == nil {
		t.Fatalf("Expected error for short key")
	}
}
