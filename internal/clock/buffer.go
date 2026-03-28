package clock

import (
	"sort"
	"sync"
	"time"
)

// LogEntry represents a distributed event with HLC timestamp.
type LogEntry struct {
	NodeID      string
	Event       string
	Transaction string
	Timestamp   Timestamp
	ReceivedAt  time.Time
}

// Buffer temporarily stores logs and flushes them in correct order.
type Buffer struct {
	entries []LogEntry
	mu      sync.Mutex
}

// NewBuffer creates a new empty reorder buffer.
func NewBuffer() *Buffer {
	return &Buffer{
		entries: []LogEntry{},
	}
}

// Add adds a log entry to the buffer.
func (b *Buffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, entry)
}

// Flush sorts the buffered entries by HLC order and returns them.
func (b *Buffer) Flush() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	sort.Slice(b.entries, func(i, j int) bool {
		if b.entries[i].Timestamp.Physical == b.entries[j].Timestamp.Physical {
			return b.entries[i].Timestamp.Logical < b.entries[j].Timestamp.Logical
		}
		return b.entries[i].Timestamp.Physical < b.entries[j].Timestamp.Physical
	})

	ordered := make([]LogEntry, len(b.entries))
	copy(ordered, b.entries)

	b.entries = []LogEntry{}
	return ordered
}

// RunPeriodicFlush flushes the buffer every interval and processes entries.
func (b *Buffer) RunPeriodicFlush(interval time.Duration, process func(LogEntry)) {
	go func() {
		for {
			time.Sleep(interval)

			ordered := b.Flush()
			for _, entry := range ordered {
				process(entry)
			}
		}
	}()
}