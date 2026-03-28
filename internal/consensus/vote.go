package consensus

import "sync"

// VoteRequest represents a request sent by a candidate asking for a vote.
type VoteRequest struct {
	CandidateID string
	Term        int
}

// VoteResponse represents a node's reply to a vote request.
type VoteResponse struct {
	VoterID     string
	Term        int
	VoteGranted bool
}

// VoteManager handles vote counting for one election round.
type VoteManager struct {
	raft *RaftState

	votes map[string]bool
	mu    sync.RWMutex
}

// NewVoteManager creates a new vote manager.
func NewVoteManager(raft *RaftState) *VoteManager {
	return &VoteManager{
		raft:  raft,
		votes: make(map[string]bool),
	}
}

// Reset clears all previously counted votes.
// Call this when a new election starts.
func (v *VoteManager) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.votes = make(map[string]bool)
}

// BuildVoteRequest creates a vote request using current candidate state.
func (v *VoteManager) BuildVoteRequest() VoteRequest {
	return VoteRequest{
		CandidateID: v.raft.NodeID,
		Term:        v.raft.GetCurrentTerm(),
	}
}

// HandleVoteRequest decides whether this node grants its vote.
func (v *VoteManager) HandleVoteRequest(req VoteRequest) VoteResponse {
	granted := false

	// Reject requests from older terms
	if req.Term < v.raft.GetCurrentTerm() {
		return VoteResponse{
			VoterID:     v.raft.NodeID,
			Term:        v.raft.GetCurrentTerm(),
			VoteGranted: false,
		}
	}

	// If request term is newer, step down to follower for that term
	if req.Term > v.raft.GetCurrentTerm() {
		v.raft.BecomeFollower(req.Term, "")
	}

	// Check if node can vote
	if v.raft.CanVote(req.CandidateID, req.Term) {
		v.raft.VoteFor(req.CandidateID)
		granted = true
	}

	return VoteResponse{
		VoterID:     v.raft.NodeID,
		Term:        v.raft.GetCurrentTerm(),
		VoteGranted: granted,
	}
}

// RecordVote stores a granted vote.
func (v *VoteManager) RecordVote(resp VoteResponse) {
	if !resp.VoteGranted {
		return
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	v.votes[resp.VoterID] = true
}

// VoteCount returns how many granted votes are recorded.
func (v *VoteManager) VoteCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return len(v.votes)
}

// HasMajority checks whether votes reach majority for a cluster size.
func (v *VoteManager) HasMajority(totalNodes int) bool {
	required := (totalNodes / 2) + 1
	return v.VoteCount() >= required
}
