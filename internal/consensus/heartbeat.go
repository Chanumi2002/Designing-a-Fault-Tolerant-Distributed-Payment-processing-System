package consensus

import (
	"log"
	"sync"
	"time"
)

// FollowerLink connects a follower raft state with its election timer.
type FollowerLink struct {
	Raft  *RaftState
	Timer *ElectionTimer
}

// HeartbeatManager sends leader heartbeats to followers.
type HeartbeatManager struct {
	leader    *RaftState
	followers map[string]FollowerLink
	interval  time.Duration

	stopCh chan struct{}
	mu     sync.RWMutex
}

// NewHeartbeatManager creates a new heartbeat manager.
func NewHeartbeatManager(leader *RaftState, interval time.Duration) *HeartbeatManager {
	return &HeartbeatManager{
		leader:    leader,
		followers: make(map[string]FollowerLink),
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// AddFollower registers a follower and its timer.
func (h *HeartbeatManager) AddFollower(nodeID string, raft *RaftState, timer *ElectionTimer) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.followers[nodeID] = FollowerLink{
		Raft:  raft,
		Timer: timer,
	}
}

// Start begins sending periodic heartbeats from leader to followers.
func (h *HeartbeatManager) Start() {
	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Only leaders should send heartbeats
				if h.leader.GetRole() != RoleLeader {
					continue
				}
				h.sendHeartbeats()

			case <-h.stopCh:
				return
			}
		}
	}()
}

// Stop stops the heartbeat loop.
func (h *HeartbeatManager) Stop() {
	select {
	case <-h.stopCh:
		// already closed
	default:
		close(h.stopCh)
	}
}

// sendHeartbeats sends one heartbeat round to all followers.
func (h *HeartbeatManager) sendHeartbeats() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	leaderID := h.leader.NodeID
	leaderTerm := h.leader.GetCurrentTerm()

	for followerID, link := range h.followers {
		link.Raft.BecomeFollower(leaderTerm, leaderID)
		link.Timer.ResetHeartbeat()
		link.Timer.ResetTimeout()

		log.Printf("Leader %s sent heartbeat to %s\n", leaderID, followerID)
	}
}