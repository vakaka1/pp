package antireplay

import (
	"testing"
	"time"
)

func TestBloomAntiReplay(t *testing.T) {
	cache := NewJTICache(1000, 0.001, 100*time.Millisecond)

	jti1 := "test-jti-1"
	jti2 := "test-jti-2"

	if !cache.CheckAndAdd(jti1) {
		t.Fatalf("first add failed")
	}

	if cache.CheckAndAdd(jti1) {
		t.Fatalf("replay not detected")
	}

	if !cache.CheckAndAdd(jti2) {
		t.Fatalf("second add failed")
	}

	time.Sleep(150 * time.Millisecond)

	jti3 := "test-jti-3"
	if !cache.CheckAndAdd(jti3) {
		t.Fatalf("add after rotation failed")
	}

	// jti1 should still be rejected as it's in the previous filter
	if cache.CheckAndAdd(jti1) {
		t.Fatalf("replay not detected after 1 rotation")
	}

	time.Sleep(150 * time.Millisecond)
	// After 2 rotations, jti1 is forgotten
	if !cache.CheckAndAdd(jti1) {
		t.Fatalf("jti1 not forgotten after 2 rotations")
	}
}
