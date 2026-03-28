package consensus

import (
	"log"
	"math/rand"
	"sync"
	"time"
)

// ElectionTimer handles follower timeout and election triggering.
type ElectionTimer struct {
	raft *RaftState

	lastHeartbeat time.Time
	timeout       time.Duration

	stopCh chan struct{}
	mu     sync.RWMutex
}

// NewElectionTimer creates a new election timer with a randomized timeout.
func NewElectionTimer(raft *RaftState) *ElectionTimer {
	return &ElectionTimer{
		raft:          raft,
		lastHeartbeat: time.Now(),
		timeout:       randomElectionTimeout(),
		stopCh:        make(chan struct{}),
	}
}

// ResetHeartbeat updates the last heartbeat time to now.
func (e *ElectionTimer) ResetHeartbeat() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.lastHeartbeat = time.Now()
}

// GetTimeout returns the current election timeout duration.
func (e *ElectionTimer) GetTimeout() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.timeout
}

// ResetTimeout generates a fresh randomized timeout.
func (e *ElectionTimer) ResetTimeout() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.timeout = randomElectionTimeout()
}

// Start begins the background timeout loop.
func (e *ElectionTimer) Start() {
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if e.shouldStartElection() {
					log.Printf("Node %s election timeout reached, becoming candidate\n", e.raft.NodeID)
					e.raft.BecomeCandidate()
					e.ResetHeartbeat()
					e.ResetTimeout()
				}
			case <-e.stopCh:
				return
			}
		}
	}()
}

// Stop stops the election timer loop.
func (e *ElectionTimer) Stop() {
	select {
	case <-e.stopCh:
		// already closed
	default:
		close(e.stopCh)
	}
}

// shouldStartElection checks whether a follower should start an election.
func (e *ElectionTimer) shouldStartElection() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Only followers should start an election in this step.
	if e.raft.GetRole() != RoleFollower {
		return false
	}

	return time.Since(e.lastHeartbeat) > e.timeout
}

// randomElectionTimeout returns a timeout between 150ms and 300ms.
func randomElectionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}