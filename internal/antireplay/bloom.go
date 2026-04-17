package antireplay

import (
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

// JTICache uses two rotating bloom filters to detect replayed JWTs.
type JTICache struct {
	current  *bloom.BloomFilter
	previous *bloom.BloomFilter
	capacity uint
	errRate  float64
	rotateAt time.Time
	interval time.Duration
	mu       sync.RWMutex
}

// NewJTICache creates a new JTICache.
func NewJTICache(capacity uint, errorRate float64, rotationInterval time.Duration) *JTICache {
	return &JTICache{
		current:  bloom.NewWithEstimates(capacity, errorRate),
		previous: bloom.NewWithEstimates(capacity, errorRate),
		capacity: capacity,
		errRate:  errorRate,
		interval: rotationInterval,
		rotateAt: time.Now().Add(rotationInterval),
	}
}

// CheckAndAdd returns false if the jti has been seen before (replayed).
// If it hasn't been seen, it adds it to the current filter and returns true.
func (c *JTICache) CheckAndAdd(jti string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if now.After(c.rotateAt) {
		c.previous = c.current
		c.current = bloom.NewWithEstimates(c.capacity, c.errRate)
		c.rotateAt = now.Add(c.interval)
	}

	jtiBytes := []byte(jti)
	if c.current.Test(jtiBytes) || c.previous.Test(jtiBytes) {
		return false // Replay
	}

	c.current.Add(jtiBytes)
	return true
}
