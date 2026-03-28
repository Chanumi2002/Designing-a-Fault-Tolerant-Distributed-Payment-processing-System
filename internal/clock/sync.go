package clock

import (
	"math/rand"
	"sync"
	"time"
)

// Clock represents a node clock with a configurable offset.
// This simulates NTP-synchronized clocks with small differences.
type Clock struct {
	offset time.Duration
	mu     sync.RWMutex
}

// NewClock creates a new clock with zero offset.
func NewClock() *Clock {
	return &Clock{
		offset: 0,
	}
}

// Now returns the current local clock time with offset applied.
func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().Add(c.offset)
}

// SetOffset manually sets the clock offset.
// Useful for testing skew between nodes.
func (c *Clock) SetOffset(offset time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = offset
}

// GetOffset returns the current clock offset.
func (c *Clock) GetOffset() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.offset
}

// Sync simulates NTP-like adjustment with a small random offset.
func (c *Clock) Sync() {
	c.mu.Lock()
	defer c.mu.Unlock()

	offset := time.Duration(rand.Intn(100)-50) * time.Millisecond
	c.offset = offset
}