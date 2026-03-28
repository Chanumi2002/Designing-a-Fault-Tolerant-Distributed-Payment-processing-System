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
	Config  *config.Config
	Client  *transport.UDPClient
}

func main() {
	cfg := config.Load()

	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/test_integration_step3 <nodeID>")
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
		Config:  cfg,
		Client:  client,
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

		case types.MsgRecoveryRequest:
			handleRecoveryRequest(app, msg)

		case types.MsgRecoveryData:
			handleRecoveryData(app, msg)
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

		time.Sleep(2 * time.Second)

		// payment A
		if app.Raft.GetRole() == consensus.RoleLeader {
			p, err := app.Service.CreatePayment("txn9001", 500.00, "user9001")
			if err != nil {
				log.Println("create payment txn9001 error:", err)
			} else {
				log.Printf("leader %s created payment A: %+v\n", app.Node.ID, p)
			}
		}

		time.Sleep(2 * time.Second)

		// simulate failure
		app.Timer.Stop()
		app.Raft.BecomeFollower(app.Raft.GetCurrentTerm(), "")
		app.Raft.SyncNodeRole(app.Node)
		log.Println("node1 simulated failure: timer stopped, stepped down from leadership")

		// rejoin later and request recovery
		time.Sleep(4 * time.Second)
		log.Println("node1 rejoined cluster as follower, requesting recovery from node2")
		requestRecoveryFromNode2(app)
	}()
}

func runNode2(app *AppNode) {
	go func() {
		time.Sleep(2 * time.Second)

		if app.Raft.GetRole() != consensus.RoleLeader {
			log.Printf("node %s is not leader before failover, payment creation skipped\n", app.Node.ID)
		}

		if !waitForPayment(app.Service, "txn9001", 90*time.Second) {
			log.Println("node2 timed out waiting for txn9001 before failover")
			return
		}

		log.Println("node2 confirmed payment A exists locally, waiting for leader failure")

		time.Sleep(2 * time.Second)

		// elect node2 after failure
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

		time.Sleep(1500 * time.Millisecond)

		// payment B
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

func requestRecoveryFromNode2(app *AppNode) {
	payload := map[string]interface{}{
		"requester_id": app.Node.ID,
	}

	data, err := types.NewMessage(types.MsgRecoveryRequest, app.Node.ID, payload)
	if err != nil {
		log.Println("build recovery request error:", err)
		return
	}

	target, ok := app.Config.FindNode("node2")
	if !ok {
		log.Println("node2 not found in config for recovery")
		return
	}

	addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	if err := app.Client.Send(addr, data); err != nil {
		log.Println("send recovery request error:", err)
		return
	}

	log.Println("node1 recovery request sent to node2")
}

func handleRecoveryRequest(app *AppNode, msg *types.Message) {
	if app.Raft.GetRole() != consensus.RoleLeader {
		return
	}

	requesterID, _ := msg.Payload["requester_id"].(string)
	if requesterID == "" {
		return
	}

	target, ok := app.Config.FindNode(requesterID)
	if !ok {
		return
	}

	payments := app.Service.GetAllPayments()

	for _, p := range payments {
		payload := map[string]interface{}{
			"transaction_id": p.TransactionID,
			"amount":         p.Amount,
			"currency":       p.Currency,
			"status":         p.Status,
			"timestamp":      p.Timestamp,
			"version":        p.Version,
			"owner_id":       p.OwnerID,
			"stripe_id":      p.StripeID,
		}

		data, err := types.NewMessage(types.MsgRecoveryData, app.Node.ID, payload)
		if err != nil {
			continue
		}

		addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
		_ = app.Client.Send(addr, data)
	}

	log.Printf("leader %s sent recovery data to %s\n", app.Node.ID, requesterID)
}

func handleRecoveryData(app *AppNode, msg *types.Message) {
	transactionID, _ := msg.Payload["transaction_id"].(string)
	currency, _ := msg.Payload["currency"].(string)
	status, _ := msg.Payload["status"].(string)
	ownerID, _ := msg.Payload["owner_id"].(string)
	stripeID, _ := msg.Payload["stripe_id"].(string)

	amount, _ := msg.Payload["amount"].(float64)
	timestamp, _ := msg.Payload["timestamp"].(float64)
	versionFloat, _ := msg.Payload["version"].(float64)
	version := int(versionFloat)

	if transactionID == "" {
		return
	}

	payment := &types.PaymentEntry{
		TransactionID: transactionID,
		Amount:        amount,
		Currency:      currency,
		Status:        status,
		Timestamp:     timestamp,
		Version:       version,
		OwnerID:       ownerID,
		StripeID:      stripeID,
	}

	if err := app.Service.ApplyRecoveredPayment(payment); err != nil {
		log.Println("apply recovered payment error:", err)
		return
	}

	log.Printf("node %s recovered payment %s\n", app.Node.ID, transactionID)
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
