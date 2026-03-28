package consensus

import (
	"sync"

	"distributed_payment_system/internal/types"
)

type RaftRole string

const (
	RoleFollower  RaftRole = "follower"
	RoleCandidate RaftRole = "candidate"
	RoleLeader    RaftRole = "leader"
)

// RaftState stores the minimum consensus state for one node.
type RaftState struct {
	NodeID string

	// Current election term
	CurrentTerm int

	// Which candidate this node voted for in the current term
	VotedFor string

	// Current raft role: follower, candidate, leader
	Role RaftRole

	// Current known leader ID
	LeaderID string

	mu sync.RWMutex
}

// NewRaftState creates the default state for a node.
func NewRaftState(nodeID string) *RaftState {
	return &RaftState{
		NodeID:      nodeID,
		CurrentTerm: 0,
		VotedFor:    "",
		Role:        RoleFollower,
		LeaderID:    "",
	}
}

// BecomeFollower switches node to follower state.
func (r *RaftState) BecomeFollower(term int, leaderID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.CurrentTerm = term
	r.Role = RoleFollower
	r.LeaderID = leaderID
	r.VotedFor = ""
}

// BecomeCandidate starts a new election term and votes for self.
func (r *RaftState) BecomeCandidate() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.CurrentTerm++
	r.Role = RoleCandidate
	r.LeaderID = ""
	r.VotedFor = r.NodeID
}

// BecomeLeader switches node to leader state.
func (r *RaftState) BecomeLeader() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Role = RoleLeader
	r.LeaderID = r.NodeID
}

// GetCurrentTerm returns the current term.
func (r *RaftState) GetCurrentTerm() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.CurrentTerm
}

// GetRole returns the current raft role.
func (r *RaftState) GetRole() RaftRole {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.Role
}

// GetLeaderID returns the current known leader.
func (r *RaftState) GetLeaderID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.LeaderID
}

// GetVotedFor returns which node this node voted for.
func (r *RaftState) GetVotedFor() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.VotedFor
}

// VoteFor stores the candidate this node voted for.
func (r *RaftState) VoteFor(candidateID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.VotedFor = candidateID
}

// CanVote checks whether the node can vote for a candidate in the current term.
func (r *RaftState) CanVote(candidateID string, term int) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if term < r.CurrentTerm {
		return false
	}

	return r.VotedFor == "" || r.VotedFor == candidateID
}

// SyncNodeRole updates the old node role field so your existing code can still use it.
func (r *RaftState) SyncNodeRole(node *types.Node) {
	if node == nil {
		return
	}

	role := r.GetRole()

	node.Mu.Lock()
	defer node.Mu.Unlock()

	switch role {
	case RoleLeader:
		node.Role = types.RoleLeader
	default:
		node.Role = types.RoleFollower
	}

	node.KnownLeader = r.GetLeaderID()
}
