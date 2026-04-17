package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

// GenerateX25519KeyPair generates a new X25519 key pair and returns the base64url encoded strings.
func GenerateX25519KeyPair() (privBase64, pubBase64 string, err error) {
	kp, err := noise.DH25519.GenerateKeypair(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate key pair: %w", err)
	}
	defer ClearBytes(kp.Private)

	privBase64 = base64.RawURLEncoding.EncodeToString(kp.Private)
	pubBase64 = base64.RawURLEncoding.EncodeToString(kp.Public)
	return privBase64, pubBase64, nil
}

// DerivePublicKey derives the public X25519 key from a base64url encoded private key.
func DerivePublicKey(privBase64 string) (string, error) {
	priv, err := DecodeKey(privBase64)
	if err != nil {
		return "", err
	}
	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, (*[32]byte)(priv))
	return base64.RawURLEncoding.EncodeToString(pub[:]), nil
}

// DecodeKey decodes a base64url encoded key string.
func DecodeKey(keyBase64 string) ([]byte, error) {
	key, err := base64.RawURLEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64url encoding: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid key length: got %d, want 32 bytes", len(key))
	}
	return key, nil
}

// ClearBytes zeroes out a byte slice to remove sensitive data from memory.
func ClearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
