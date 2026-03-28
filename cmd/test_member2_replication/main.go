package main

import (
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/replication"
	"distributed_payment_system/internal/storage"
	"distributed_payment_system/internal/transport"
	"distributed_payment_system/internal/types"
)

func main() {
	cfg := config.Load()

	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/test_member2_replication <nodeID>")
	}

	nodeID := os.Args[1]

	nodeCfg, ok := cfg.FindNode(nodeID)
	if !ok {
		log.Fatalf("node %s not found\n", nodeID)
	}

	node := types.NewNode(nodeCfg.ID, nodeCfg.Host, nodeCfg.Port)

	if node.ID == "node1" {
		node.Role = types.RoleLeader
		node.KnownLeader = node.ID
	} else {
		node.Role = types.RoleFollower
		node.KnownLeader = "node1"
	}

	client := transport.NewUDPClient()
	store := storage.NewMemoryStore()
	service := replication.NewService(store, node, cfg, client)

	address := net.JoinHostPort(node.Host, strconv.Itoa(node.Port))

	server := transport.NewUDPServer(address, func(msg *types.Message) {
		switch msg.Type {
		case types.MsgPaymentReplicate:
			if err := service.HandleReplicatedPayment(msg); err != nil {
				log.Println("replication apply error:", err)
			} else {
				txID, _ := msg.Payload["transaction_id"].(string)
				log.Printf("node %s stored replicated payment %s\n", node.ID, txID)
			}

		case types.MsgPaymentAck:
			if node.Role == types.RoleLeader {
				service.HandlePaymentAck(msg)

				txID, _ := msg.Payload["transaction_id"].(string)
				count := service.GetAckCount(txID)

				log.Printf("leader received ACK for %s, count=%d\n", txID, count)

				if service.HasMajority(txID) {
					service.MarkCommitted(txID)
					log.Printf("payment %s reached majority and is now committed\n", txID)
				}
			}
		}
	})

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	log.Printf("node %s listening on %s as %s\n", node.ID, address, node.Role)

	if node.Role == types.RoleLeader {
		go func() {
			time.Sleep(3 * time.Second)

			log.Println("TEST 1: create txn2001 first time")
			p1, err := service.CreatePayment("txn2001", 300.00, "user2001")
			if err != nil {
				log.Println("first create payment error:", err)
			} else {
				log.Printf("leader created payment first time: %+v\n", p1)
			}

			time.Sleep(2 * time.Second)

			log.Println("TEST 2: create txn2001 second time")
			p2, err := service.CreatePayment("txn2001", 300.00, "user2001")
			if err != nil {
				log.Println("duplicate detected correctly on leader:", err)
			} else {
				log.Printf("unexpected second create success: %+v\n", p2)
			}
		}()
	}

	select {}
}
