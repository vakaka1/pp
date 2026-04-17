package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GeneratePSK generates a 32-byte pre-shared key (PSK) and returns its base64url encoded string.
func GeneratePSK() (string, error) {
	psk := make([]byte, 32)
	_, err := rand.Read(psk)
	if err != nil {
		return "", fmt.Errorf("failed to generate PSK: %w", err)
	}
	defer ClearBytes(psk)
	return base64.RawURLEncoding.EncodeToString(psk), nil
}
