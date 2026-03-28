package clock

import "sync"

// Timestamp represents a Hybrid Logical Clock timestamp.
type Timestamp struct {
	Physical int64
	Logical  int64
}

// HLC keeps track of the latest hybrid timestamp.
type HLC struct {
	last  Timestamp
	mu    sync.Mutex
	clock *Clock
}

// NewHLC creates a new HLC using the provided clock.
func NewHLC(c *Clock) *HLC {
	return &HLC{
		last:  Timestamp{},
		clock: c,
	}
}

// Now generates an HLC timestamp for a local event.
func (h *HLC) Now() Timestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := h.clock.Now().UnixNano()

	if now > h.last.Physical {
		h.last.Physical = now
		h.last.Logical = 0
	} else {
		h.last.Logical++
	}

	return h.last
}

// Update merges a remote timestamp into the local HLC state.
func (h *HLC) Update(remote Timestamp) Timestamp {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := h.clock.Now().UnixNano()
	physical := maxInt64(now, maxInt64(h.last.Physical, remote.Physical))

	if physical == h.last.Physical && physical == remote.Physical {
		h.last.Logical = maxInt64(h.last.Logical, remote.Logical) + 1
	} else if physical == h.last.Physical {
		h.last.Logical++
	} else if physical == remote.Physical {
		h.last.Logical = remote.Logical + 1
	} else {
		h.last.Logical = 0
	}

	h.last.Physical = physical
	return h.last
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}