package protocol

import (
	"testing"
	"time"
)

func TestJWTGenerateAndValidate(t *testing.T) {
	psk := make([]byte, 32)
	for i := range psk {
		psk[i] = byte(i)
	}

	iat := time.Now()
	exp := iat.Add(10 * time.Minute)
	jti := "test-jti-12345"
	sub := "test-sub"

	tokenStr, err := GenerateJWT(psk, jti, sub, iat, exp)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	seen := make(map[string]bool)
	checkJTI := func(j string) bool {
		if seen[j] {
			return false
		}
		seen[j] = true
		return true
	}

	valid, err := ValidateJWT(tokenStr, psk, 15*time.Minute, checkJTI)
	if err != nil || !valid {
		t.Fatalf("ValidateJWT failed: %v", err)
	}

	// Replay test
	valid, err = ValidateJWT(tokenStr, psk, 15*time.Minute, checkJTI)
	if valid || err == nil {
		t.Fatalf("ValidateJWT should fail on replay")
	}
}

func TestJWTTimingAttack(t *testing.T) {
	// Simple test to ensure TimingSafeCompare works as expected.
	a := []byte("secret")
	b := []byte("secret")
	if !TimingSafeCompare(a, b) {
		t.Fatalf("TimingSafeCompare failed for equal")
	}
	c := []byte("secres")
	if TimingSafeCompare(a, c) {
		t.Fatalf("TimingSafeCompare failed for unequal")
	}
}
