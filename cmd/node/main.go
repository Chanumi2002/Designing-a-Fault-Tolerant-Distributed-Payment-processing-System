package main

import (
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/fault"
	"distributed_payment_system/internal/transport"
	"distributed_payment_system/internal/types"
)

func main() {
	cfg := config.Load()

	// Expect node ID from command line
	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/node <nodeID>")
	}

	nodeID := os.Args[1]

	nodeCfg, ok := cfg.FindNode(nodeID)
	if !ok {
		log.Fatalf("node %s not found in config\n", nodeID)
	}

	node := types.NewNode(nodeCfg.ID, nodeCfg.Host, nodeCfg.Port)

	// Simple startup leader assignment for testing
	if node.ID == "node1" {
		node.Role = types.RoleLeader
		node.KnownLeader = node.ID

		// Sample payment data on leader for recovery testing
		node.Payments["txn1001"] = &types.PaymentEntry{
			TransactionID: "txn1001",
			Amount:        120.50,
			Currency:      "USD",
			Status:        "completed",
			Timestamp:     float64(time.Now().UnixNano()) / 1e9,
			Version:       1,
			OwnerID:       "user1",
			StripeID:      "stripe_001",
		}

		node.Payments["txn1002"] = &types.PaymentEntry{
			TransactionID: "txn1002",
			Amount:        75.00,
			Currency:      "USD",
			Status:        "completed",
			Timestamp:     float64(time.Now().UnixNano()) / 1e9,
			Version:       1,
			OwnerID:       "user2",
			StripeID:      "stripe_002",
		}
	} else {
		node.Role = types.RoleFollower
		node.KnownLeader = "node1"
	}

	client := transport.NewUDPClient()
	detector := fault.NewDetector(node, cfg, client)
	recovery := fault.NewRecoveryManager(node, cfg, client)

	address := net.JoinHostPort(node.Host, strconv.Itoa(node.Port))

	server := transport.NewUDPServer(address, func(msg *types.Message) {
		switch msg.Type {
		case types.MsgHeartbeat:
			detector.HandleHeartbeat(msg)

		case types.MsgRecoveryRequest:
			// Only leader responds with recovery data
			if node.Role == types.RoleLeader {
				if err := recovery.HandleRecoveryRequest(msg); err != nil {
					log.Println("handle recovery request error:", err)
				}
			}

		case types.MsgRecoveryData:
			recovery.HandleRecoveryData(msg)

		default:
			log.Printf("node %s received message type %s from %s\n", node.ID, msg.Type, msg.Sender)
		}
	})

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	detector.Start()

	// Followers request recovery once after startup
	go func() {
		time.Sleep(3 * time.Second)
		if node.Role == types.RoleFollower {
			if err := recovery.RequestRecovery(); err != nil {
				log.Println("recovery request error:", err)
			}
		}
	}()

	// Print peer statuses every 5 seconds for debugging
	go func() {
		for {
			time.Sleep(5 * time.Second)
			detector.PrintStatuses()
		}
	}()

	log.Printf("node %s started on %s as %s\n", node.ID, address, node.Role)

	select {}

	// if node.ID == "node1" {
	// 	node.Role = types.RoleLeader
	// 	node.KnownLeader = node.ID

	// 	node.Payments["txn1001"] = &types.PaymentEntry{
	// 		TransactionID: "txn1001",
	// 		Amount:        120.50,
	// 		Currency:      "USD",
	// 		Status:        "completed",
	// 		Timestamp:     float64(time.Now().UnixNano()) / 1e9,
	// 		Version:       1,
	// 		OwnerID:       "user1",
	// 		StripeID:      "stripe_001",
	// 	}

	// 	node.Payments["txn1002"] = &types.PaymentEntry{
	// 		TransactionID: "txn1002",
	// 		Amount:        75.00,
	// 		Currency:      "USD",
	// 		Status:        "completed",
	// 		Timestamp:     float64(time.Now().UnixNano()) / 1e9,
	// 		Version:       1,
	// 		OwnerID:       "user2",
	// 		StripeID:      "stripe_002",
	// 	}
	// } else {
	// 	node.Role = types.RoleFollower
	// 	node.KnownLeader = "node1"
	// }
}
