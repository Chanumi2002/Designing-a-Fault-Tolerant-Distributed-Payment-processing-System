package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	clockpkg "distributed_payment_system/internal/clock"
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

	Clock  *clockpkg.Clock
	HLC    *clockpkg.HLC
	Buffer *clockpkg.Buffer
}

func main() {
	cfg := config.Load()

	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/test_integration_final <nodeID> [normal|rejoin]")
	}

	nodeID := os.Args[1]
	mode := "normal"
	if len(os.Args) >= 3 {
		mode = os.Args[2]
	}

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

	localClock := clockpkg.NewClock()
	localClock.SetOffset(getNodeOffset(node.ID))

	app := &AppNode{
		Node:    node,
		Raft:    raft,
		Timer:   timer,
		Service: service,
		Config:  cfg,
		Client:  client,
		Clock:   localClock,
		HLC:     clockpkg.NewHLC(localClock),
		Buffer:  clockpkg.NewBuffer(),
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
				recordEvent(app, "replicated payment received", txID)
			}

		case types.MsgPaymentAck:
			if app.Raft.GetRole() == consensus.RoleLeader {
				app.Service.HandlePaymentAck(msg)

				txID, _ := msg.Payload["transaction_id"].(string)
				count := app.Service.GetAckCount(txID)

				log.Printf("leader %s received ACK for %s, count=%d\n", app.Node.ID, txID, count)
				recordEvent(app, "ACK received", txID)

				payment, ok := app.Service.GetPayment(txID)
				if ok && payment.Status != "committed" && app.Service.HasMajority(txID) {
					app.Service.MarkCommitted(txID)
					log.Printf("payment %s reached majority and is now committed by leader %s\n", txID, app.Node.ID)
					recordEvent(app, "payment committed", txID)
				}
			}

		case types.MsgRecoveryRequest:
			recordEvent(app, "recovery request received", "")
			handleRecoveryRequest(app, msg)

		case types.MsgRecoveryData:
			handleRecoveryData(app, msg)
		}
	})

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	log.Printf("node %s listening on %s\n", app.Node.ID, address)

	logClockInfo(app)
	startOrderedLogPrinter(app)
	recordEvent(app, "node started", "")

	switch app.Node.ID {
	case "node1":
		if mode == "rejoin" {
			runNode1Rejoin(app)
		} else {
			runNode1Normal(app)
		}
	case "node2":
		runNode2(app)
	case "node3":
		runNode3(app)
	}

	select {}
}

func runNode1Normal(app *AppNode) {
	go func() {
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
				recordEvent(app, "leader elected", "")
			}
		}

		time.Sleep(2 * time.Second)

		if app.Raft.GetRole() == consensus.RoleLeader {
			p, err := app.Service.CreatePayment("txn9001", 500.00, "user9001")
			if err != nil {
				log.Println("create payment txn9001 error:", err)
			} else {
				log.Printf("leader %s created payment A: %+v\n", app.Node.ID, p)
				recordEvent(app, "payment created", p.TransactionID)
			}
		}

		time.Sleep(2 * time.Second)

		app.Timer.Stop()
		app.Raft.BecomeFollower(app.Raft.GetCurrentTerm(), "")
		app.Raft.SyncNodeRole(app.Node)
		log.Println("node1 simulated failure: timer stopped, stepped down from leadership")
		recordEvent(app, "leader failed", "")

		time.Sleep(4 * time.Second)
		log.Println("node1 rejoined cluster as follower, requesting recovery from node2")
		recordEvent(app, "node rejoined", "")
		requestRecoveryFromNode2(app)
	}()
}

func runNode1Rejoin(app *AppNode) {
	go func() {
		app.Raft.BecomeFollower(app.Raft.GetCurrentTerm(), "node2")
		app.Raft.SyncNodeRole(app.Node)

		log.Println("node1 started in rejoin mode as follower")
		recordEvent(app, "node rejoined", "")

		time.Sleep(2 * time.Second)
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
		recordEvent(app, "payment visible on follower", "txn9001")

		time.Sleep(2 * time.Second)

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
				recordEvent(app, "new leader elected", "")
			}
		}

		time.Sleep(1500 * time.Millisecond)

		if app.Raft.GetRole() == consensus.RoleLeader {
			p, err := app.Service.CreatePayment("txn9002", 750.00, "user9002")
			if err != nil {
				log.Println("create payment txn9002 error:", err)
			} else {
				log.Printf("leader %s created payment B: %+v\n", app.Node.ID, p)
				recordEvent(app, "payment created", p.TransactionID)
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
	ts := app.HLC.Now()

	payload := map[string]interface{}{
		"requester_id":   app.Node.ID,
		"hlc_physical":   ts.Physical,
		"hlc_logical":    ts.Logical,
		"requester_time": app.Clock.Now().Format(time.RFC3339Nano),
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

	log.Printf("%s recovery request sent to node2\n", app.Node.ID)
	recordEvent(app, "recovery request sent", "")
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
		ts := app.HLC.Now()

		payload := map[string]interface{}{
			"transaction_id": p.TransactionID,
			"amount":         p.Amount,
			"currency":       p.Currency,
			"status":         p.Status,
			"timestamp":      p.Timestamp,
			"version":        p.Version,
			"owner_id":       p.OwnerID,
			"stripe_id":      p.StripeID,
			"hlc_physical":   ts.Physical,
			"hlc_logical":    ts.Logical,
		}

		data, err := types.NewMessage(types.MsgRecoveryData, app.Node.ID, payload)
		if err != nil {
			continue
		}

		addr := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
		_ = app.Client.Send(addr, data)
		recordEvent(app, "recovery data sent", p.TransactionID)
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

	if physical, ok := toInt64(msg.Payload["hlc_physical"]); ok {
		if logical, ok2 := toInt64(msg.Payload["hlc_logical"]); ok2 {
			app.HLC.Update(clockpkg.Timestamp{
				Physical: physical,
				Logical:  logical,
			})
		}
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

	alreadyHad := false
	if _, ok := app.Service.GetPayment(transactionID); ok {
		alreadyHad = true
	}

	if err := app.Service.ApplyRecoveredPayment(payment); err != nil {
		log.Println("apply recovered payment error:", err)
		return
	}

	if alreadyHad {
		log.Printf("node %s already had payment %s before recovery sync\n", app.Node.ID, transactionID)
		recordEvent(app, "recovery sync checked existing payment", transactionID)
		return
	}

	log.Printf("node %s recovered payment %s\n", app.Node.ID, transactionID)
	recordEvent(app, "recovery applied", transactionID)
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

func getNodeOffset(nodeID string) time.Duration {
	switch nodeID {
	case "node1":
		return 0 * time.Millisecond
	case "node2":
		return 50 * time.Millisecond
	case "node3":
		return -30 * time.Millisecond
	default:
		return 0
	}
}

func logClockInfo(app *AppNode) {
	log.Printf("node %s local time: %s\n", app.Node.ID, app.Clock.Now().Format(time.RFC3339Nano))

	for _, other := range app.Config.Nodes {
		if other.ID == app.Node.ID {
			continue
		}

		otherClock := clockpkg.NewClock()
		otherClock.SetOffset(getNodeOffset(other.ID))

		clockpkg.CheckNodeSkew(
			fmt.Sprintf("%s vs %s", other.ID, app.Node.ID),
			app.Clock.Now(),
			otherClock.Now(),
		)
	}
}

func startOrderedLogPrinter(app *AppNode) {
	app.Buffer.RunPeriodicFlush(2*time.Second, func(entry clockpkg.LogEntry) {
		log.Printf(
			"[ORDERED] node=%s event=%s txn=%s physical=%d logical=%d\n",
			entry.NodeID,
			entry.Event,
			entry.Transaction,
			entry.Timestamp.Physical,
			entry.Timestamp.Logical,
		)
	})
}

func recordEvent(app *AppNode, event, txn string) {
	if app == nil || app.Buffer == nil || app.HLC == nil {
		return
	}

	ts := app.HLC.Now()

	app.Buffer.Add(clockpkg.LogEntry{
		NodeID:      app.Node.ID,
		Event:       event,
		Transaction: txn,
		Timestamp:   ts,
		ReceivedAt:  app.Clock.Now(),
	})
}

func toInt64(v interface{}) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case float64:
		return int64(t), true
	default:
		return 0, false
	}
}
