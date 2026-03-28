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
		log.Fatal("usage: go run ./cmd/test_integration_step2 <nodeID>")
	}

	nodeID := os.Args[1]
	nodeCfg, ok := cfg.FindNode(nodeID)
	if !ok {
		log.Fatalf("node %s not found\n", nodeID)
	}

	node := types.NewNode(nodeCfg.ID, nodeCfg.Host, nodeCfg.Port)
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

	switch app.Node.ID {
	case "node1":
		runNode1(app)
	case "node2":
		runNode2(app)
	case "node3":
		runNode3(app)
	}

	select {}
}

func runNode1(app *AppNode) {
	go func() {
		// Phase 1: elect node1 as leader
		app.Timer.Start()
		time.Sleep(500 * time.Millisecond)

		if app.Raft.GetRole() == consensus.RoleCandidate {
			vm := consensus.NewVoteManager(app.Raft)
			vm.Reset()

			vm.RecordVote(consensus.VoteResponse{
				VoterID:     "node1",
				Term:        app.Raft.GetCurrentTerm(),
				VoteGranted: true,
			})

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

		// Give followers time to be ready
		time.Sleep(2 * time.Second)

		// Phase 2: payment A under node1
		if app.Raft.GetRole() == consensus.RoleLeader {
			p, err := app.Service.CreatePayment("txn9001", 500.00, "user9001")
			if err != nil {
				log.Println("create payment txn9001 error:", err)
			} else {
				log.Printf("leader %s created payment A: %+v\n", app.Node.ID, p)
			}
		}

		// Wait before simulating failure
		time.Sleep(2 * time.Second)

		// Phase 3: simulate node1 failure
		app.Timer.Stop()
		app.Raft.BecomeFollower(app.Raft.GetCurrentTerm(), "")
		app.Raft.SyncNodeRole(app.Node)
		log.Println("node1 simulated failure: timer stopped, stepped down from leadership")
	}()
}

func runNode2(app *AppNode) {
	go func() {
		time.Sleep(2 * time.Second)

		if app.Raft.GetRole() != consensus.RoleLeader {
			log.Printf("node %s is not leader before failover, payment creation skipped\n", app.Node.ID)
		}

		// Wait until payment A exists locally
		if !waitForPayment(app.Service, "txn9001", 90*time.Second) {
			log.Println("node2 timed out waiting for txn9001 before failover")
			return
		}

		log.Println("node2 confirmed payment A exists locally, waiting for leader failure")

		// Give node1 time to fail
		time.Sleep(2 * time.Second)

		// Start failover election
		app.Timer.Start()
		time.Sleep(500 * time.Millisecond)

		if app.Raft.GetRole() == consensus.RoleCandidate {
			vm := consensus.NewVoteManager(app.Raft)
			vm.Reset()

			vm.RecordVote(consensus.VoteResponse{
				VoterID:     "node2",
				Term:        app.Raft.GetCurrentTerm(),
				VoteGranted: true,
			})

			n3 := consensus.NewRaftState("node3")
			req := vm.BuildVoteRequest()
			resp3 := consensus.NewVoteManager(n3).HandleVoteRequest(req)
			vm.RecordVote(resp3)

			log.Printf("node2 failover vote from node3 -> granted=%v term=%d\n", resp3.VoteGranted, resp3.Term)
			log.Printf("node2 total votes after failover: %d\n", vm.VoteCount())

			if vm.HasMajority(3) {
				app.Raft.BecomeLeader()
				app.Raft.SyncNodeRole(app.Node)
				log.Printf("Raft elected %s as new leader in term %d\n", app.Node.ID, app.Raft.GetCurrentTerm())
			}
		}

		// Phase 4: payment B under new leader node2
		time.Sleep(1500 * time.Millisecond)

		if app.Raft.GetRole() == consensus.RoleLeader {
			p, err := app.Service.CreatePayment("txn9002", 750.00, "user9002")
			if err != nil {
				log.Println("create payment txn9002 error:", err)
			} else {
				log.Printf("leader %s created payment B: %+v\n", app.Node.ID, p)
			}
		}
	}()
}

func runNode3(app *AppNode) {
	go func() {
		time.Sleep(2 * time.Second)

		if app.Raft.GetRole() != consensus.RoleLeader {
			log.Printf("node %s is not leader, payment creation skipped\n", app.Node.ID)
		}
	}()
}

func waitForPayment(service *replication.Service, transactionID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, ok := service.GetPayment(transactionID); ok {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}

	return false
}
