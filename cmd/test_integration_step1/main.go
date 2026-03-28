package main

import (
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/consensus"
	"distributed_payment_system/internal/replication"
	"distributed_payment_system/internal/storage"
	"distributed_payment_system/internal/transport"
	"distributed_payment_system/internal/types"
)

type AppNode struct {
	Node    *types.Node
	Raft    *consensus.RaftState
	Timer   *consensus.ElectionTimer
	Service *replication.Service
}

func main() {
	cfg := config.Load()

	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/test_integration_step1 <nodeID>")
	}

	nodeID := os.Args[1]

	nodeCfg, ok := cfg.FindNode(nodeID)
	if !ok {
		log.Fatalf("node %s not found\n", nodeID)
	}

	// Base node
	node := types.NewNode(nodeCfg.ID, nodeCfg.Host, nodeCfg.Port)

	// Member 4: real leader info comes from Raft, not hardcoded role
	raft := consensus.NewRaftState(node.ID)
	timer := consensus.NewElectionTimer(raft)

	client := transport.NewUDPClient()
	store := storage.NewMemoryStore()
	service := replication.NewService(store, node, cfg, client)

	app := &AppNode{
		Node:    node,
		Raft:    raft,
		Timer:   timer,
		Service: service,
	}

	address := net.JoinHostPort(node.Host, strconv.Itoa(node.Port))

	server := transport.NewUDPServer(address, func(msg *types.Message) {
		switch msg.Type {
		case types.MsgPaymentReplicate:
			if err := app.Service.HandleReplicatedPayment(msg); err != nil {
				log.Println("replication apply error:", err)
			} else {
				txID, _ := msg.Payload["transaction_id"].(string)
				log.Printf("node %s stored replicated payment %s\n", app.Node.ID, txID)
			}

		case types.MsgPaymentAck:
			// Member 2 now checks Member 4 role
			if app.Raft.GetRole() == consensus.RoleLeader {
				app.Service.HandlePaymentAck(msg)

				txID, _ := msg.Payload["transaction_id"].(string)
				count := app.Service.GetAckCount(txID)

				log.Printf("leader %s received ACK for %s, count=%d\n", app.Node.ID, txID, count)

				payment, ok := app.Service.GetPayment(txID)
				if ok && payment.Status != "committed" && app.Service.HasMajority(txID) {
					app.Service.MarkCommitted(txID)
					log.Printf("payment %s reached majority and is now committed by leader %s\n", txID, app.Node.ID)
				}
			}
		}
	})

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	log.Printf("node %s listening on %s\n", app.Node.ID, address)

	// ----------------------------
	// Controlled election for step 1
	// ----------------------------
	// Start only node1 timer so node1 becomes leader by Raft election,
	// not by hardcoded assignment.
	if app.Node.ID == "node1" {
		app.Timer.Start()

		go func() {
			time.Sleep(500 * time.Millisecond)

			// node1 times out -> candidate
			if app.Raft.GetRole() == consensus.RoleCandidate {
				vm := consensus.NewVoteManager(app.Raft)
				vm.Reset()

				// self vote
				vm.RecordVote(consensus.VoteResponse{
					VoterID:     "node1",
					Term:        app.Raft.GetCurrentTerm(),
					VoteGranted: true,
				})

				// Simulate node2 + node3 votes
				n2 := consensus.NewRaftState("node2")
				n3 := consensus.NewRaftState("node3")

				req := vm.BuildVoteRequest()
				resp2 := consensus.NewVoteManager(n2).HandleVoteRequest(req)
				resp3 := consensus.NewVoteManager(n3).HandleVoteRequest(req)

				vm.RecordVote(resp2)
				vm.RecordVote(resp3)

				if vm.HasMajority(3) {
					app.Raft.BecomeLeader()
					app.Raft.SyncNodeRole(app.Node)
					log.Printf("Raft elected %s as leader in term %d\n", app.Node.ID, app.Raft.GetCurrentTerm())
				}
			}
		}()
	}

	// ----------------------------
	// Member 2 now obeys Member 4
	// ----------------------------
	// Try creating a payment only if this node is elected leader.
	go func() {
		time.Sleep(2 * time.Second)

		if app.Raft.GetRole() != consensus.RoleLeader {
			log.Printf("node %s is not leader, payment creation skipped\n", app.Node.ID)
			return
		}

		p, err := app.Service.CreatePayment("txn9001", 500.00, "user9001")
		if err != nil {
			log.Println("create payment error:", err)
			return
		}

		log.Printf("leader %s created payment: %+v\n", app.Node.ID, p)
	}()

	select {}
}
