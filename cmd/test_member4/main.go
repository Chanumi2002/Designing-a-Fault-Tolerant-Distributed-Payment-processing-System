package main

import (
	"log"
	"time"

	"distributed_payment_system/internal/consensus"
)

func main() {
	// Create 3 raft nodes
	node1 := consensus.NewRaftState("node1")
	node2 := consensus.NewRaftState("node2")
	node3 := consensus.NewRaftState("node3")

	// Election timers
	timer1 := consensus.NewElectionTimer(node1)
	timer2 := consensus.NewElectionTimer(node2)
	timer3 := consensus.NewElectionTimer(node3)

	log.Println("===== MEMBER 4 STEP 5 TEST START =====")
	log.Printf("Initial -> node1=%s node2=%s node3=%s\n", node1.GetRole(), node2.GetRole(), node3.GetRole())

	// -----------------------------
	// PHASE 1: Elect node1 as leader
	// -----------------------------
	timer1.Start()

	time.Sleep(500 * time.Millisecond)

	log.Println("===== PHASE 1: NODE1 ELECTION =====")
	log.Printf("After timeout -> node1 role=%s term=%d votedFor=%s\n", node1.GetRole(), node1.GetCurrentTerm(), node1.GetVotedFor())

	if node1.GetRole() == consensus.RoleCandidate {
		vm1 := consensus.NewVoteManager(node1)
		vm1.Reset()

		// node1 votes for itself
		vm1.RecordVote(consensus.VoteResponse{
			VoterID:     "node1",
			Term:        node1.GetCurrentTerm(),
			VoteGranted: true,
		})

		req := vm1.BuildVoteRequest()

		resp2 := consensus.NewVoteManager(node2).HandleVoteRequest(req)
		resp3 := consensus.NewVoteManager(node3).HandleVoteRequest(req)

		log.Printf("Vote for node1 from node2 -> granted=%v term=%d\n", resp2.VoteGranted, resp2.Term)
		log.Printf("Vote for node1 from node3 -> granted=%v term=%d\n", resp3.VoteGranted, resp3.Term)

		vm1.RecordVote(resp2)
		vm1.RecordVote(resp3)

		log.Printf("Node1 total votes: %d\n", vm1.VoteCount())

		if vm1.HasMajority(3) {
			node1.BecomeLeader()
			log.Printf("Node1 became leader in term %d\n", node1.GetCurrentTerm())
		}
	}

	// Start leader heartbeat from node1
	hb1 := consensus.NewHeartbeatManager(node1, 100*time.Millisecond)
	hb1.AddFollower("node2", node2, timer2)
	hb1.AddFollower("node3", node3, timer3)
	hb1.Start()

	// Let node1 heartbeat stabilize followers
	time.Sleep(500 * time.Millisecond)

	log.Println("===== PHASE 2: NODE1 HEARTBEAT ACTIVE =====")
	log.Printf("node1 role=%s leaderID=%s term=%d\n", node1.GetRole(), node1.GetLeaderID(), node1.GetCurrentTerm())
	log.Printf("node2 role=%s leaderID=%s term=%d\n", node2.GetRole(), node2.GetLeaderID(), node2.GetCurrentTerm())
	log.Printf("node3 role=%s leaderID=%s term=%d\n", node3.GetRole(), node3.GetLeaderID(), node3.GetCurrentTerm())

	// --------------------------------------
	// PHASE 3: Simulate node1 leader failure
	// --------------------------------------
	log.Println("===== PHASE 3: SIMULATE NODE1 FAILURE =====")
	hb1.Stop()
	timer1.Stop()

	// Simulate node1 crash by stopping its heartbeat and election timer.
	// It should not participate in the next election.
	node1.BecomeFollower(node1.GetCurrentTerm(), "")

	log.Println("Node1 heartbeat stopped and node1 timer stopped (leader failure simulated)")

	// Start node2 timer for controlled failover election
	timer2.Start()

	// Wait for node2 timeout
	time.Sleep(500 * time.Millisecond)

	log.Println("===== PHASE 4: NODE2 FAILOVER ELECTION =====")
	log.Printf("node1 role=%s term=%d votedFor=%s leaderID=%s\n", node1.GetRole(), node1.GetCurrentTerm(), node1.GetVotedFor(), node1.GetLeaderID())
	log.Printf("node2 role=%s term=%d votedFor=%s leaderID=%s\n", node2.GetRole(), node2.GetCurrentTerm(), node2.GetVotedFor(), node2.GetLeaderID())
	log.Printf("node3 role=%s term=%d votedFor=%s leaderID=%s\n", node3.GetRole(), node3.GetCurrentTerm(), node3.GetVotedFor(), node3.GetLeaderID())

	if node2.GetRole() == consensus.RoleCandidate {
		vm2 := consensus.NewVoteManager(node2)
		vm2.Reset()

		// node2 votes for itself
		vm2.RecordVote(consensus.VoteResponse{
			VoterID:     "node2",
			Term:        node2.GetCurrentTerm(),
			VoteGranted: true,
		})

		req := vm2.BuildVoteRequest()

		// Simulate node3 voting for node2
		resp3 := consensus.NewVoteManager(node3).HandleVoteRequest(req)
		log.Printf("Vote for node2 from node3 -> granted=%v term=%d\n", resp3.VoteGranted, resp3.Term)

		vm2.RecordVote(resp3)

		log.Printf("Node2 total votes: %d\n", vm2.VoteCount())

		if vm2.HasMajority(3) {
			node2.BecomeLeader()
			log.Printf("Node2 became new leader in term %d\n", node2.GetCurrentTerm())
		}
	}

	// Start new leader heartbeat from node2
	if node2.GetRole() == consensus.RoleLeader {
		hb2 := consensus.NewHeartbeatManager(node2, 100*time.Millisecond)
		hb2.AddFollower("node1", node1, timer1)
		hb2.AddFollower("node3", node3, timer3)
		hb2.Start()

		time.Sleep(500 * time.Millisecond)
		hb2.Stop()
	}

	log.Println("===== FINAL STATE =====")
	log.Printf("node1 role=%s term=%d leaderID=%s votedFor=%s\n", node1.GetRole(), node1.GetCurrentTerm(), node1.GetLeaderID(), node1.GetVotedFor())
	log.Printf("node2 role=%s term=%d leaderID=%s votedFor=%s\n", node2.GetRole(), node2.GetCurrentTerm(), node2.GetLeaderID(), node2.GetVotedFor())
	log.Printf("node3 role=%s term=%d leaderID=%s votedFor=%s\n", node3.GetRole(), node3.GetCurrentTerm(), node3.GetLeaderID(), node3.GetVotedFor())

	log.Println("===== MEMBER 4 STEP 5 TEST END =====")
}
