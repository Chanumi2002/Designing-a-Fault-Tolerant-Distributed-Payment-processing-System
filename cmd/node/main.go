package main

import (
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/fault"
	"distributed_payment_system/internal/replication"
	"distributed_payment_system/internal/transport"
	"distributed_payment_system/internal/types"
	"distributed_payment_system/internal/utils"
)

type nodeStore struct {
	node *types.Node
}

func (s *nodeStore) HasPayment(transactionID string) bool {
	s.node.Mu.RLock()
	defer s.node.Mu.RUnlock()

	_, ok := s.node.Payments[transactionID]
	return ok
}

func (s *nodeStore) SavePayment(payment *types.PaymentEntry) error {
	if payment == nil || payment.TransactionID == "" {
		return nil
	}

	s.node.Mu.Lock()
	defer s.node.Mu.Unlock()

	cp := *payment
	s.node.Payments[payment.TransactionID] = &cp
	return nil
}

func (s *nodeStore) GetPayment(transactionID string) (*types.PaymentEntry, bool) {
	s.node.Mu.RLock()
	defer s.node.Mu.RUnlock()

	p, ok := s.node.Payments[transactionID]
	if !ok {
		return nil, false
	}

	cp := *p
	return &cp, true
}

func (s *nodeStore) GetAllPayments() []*types.PaymentEntry {
	s.node.Mu.RLock()
	defer s.node.Mu.RUnlock()

	result := make([]*types.PaymentEntry, 0, len(s.node.Payments))
	for _, p := range s.node.Payments {
		cp := *p
		result = append(result, &cp)
	}
	return result
}

func main() {
	cfg := config.Load()

	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/node <nodeID>")
	}

	nodeID := strings.TrimSpace(os.Args[1])

	logFile, err := utils.SetupGlobalFileLogger(nodeID + ".log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	nodeCfg, ok := cfg.FindNode(nodeID)
	if !ok {
		log.Fatalf("node %s not found in config\n", nodeID)
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
	store := &nodeStore{node: node}
	replicationService := replication.NewService(store, node, cfg, client)
	detector := fault.NewDetector(node, cfg, client)
	recovery := fault.NewRecoveryManager(node, cfg, client)

	address := net.JoinHostPort(node.Host, strconv.Itoa(node.Port))

	server := transport.NewUDPServer(address, func(msg *types.Message) {
		switch msg.Type {
		case types.MsgHeartbeat:
			detector.HandleHeartbeat(msg)

		case types.MsgPaymentCreate:
			if node.Role == types.RoleLeader {
				transactionID, _ := msg.Payload["transaction_id"].(string)
				ownerID, _ := msg.Payload["owner_id"].(string)
				amount, _ := msg.Payload["amount"].(float64)

				payment, err := replicationService.CreatePayment(transactionID, amount, ownerID)
				if err != nil || payment == nil {
					return
				}

				go func(txn string) {
					timeout := time.After(5 * time.Second)
					ticker := time.NewTicker(100 * time.Millisecond)
					defer ticker.Stop()

					for {
						select {
						case <-timeout:
							return
						case <-ticker.C:
							if replicationService.HasMajority(txn) {
								replicationService.MarkCommitted(txn)
								_ = replicationService.BroadcastCommit(txn)
								return
							}
						}
					}
				}(payment.TransactionID)
			}

		case types.MsgPaymentReplicate:
			_ = replicationService.HandleReplicatedPayment(msg)

		case types.MsgPaymentAck:
			replicationService.HandlePaymentAck(msg)

		case types.MsgPaymentCommit:
			_ = replicationService.HandleCommitPayment(msg)

		case types.MsgRecoveryRequest:
			if node.Role == types.RoleLeader {
				if err := recovery.HandleRecoveryRequest(msg); err != nil {
					log.Println("handle recovery request error:", err)
				}
			}

		case types.MsgRecoveryData:
			recovery.HandleRecoveryData(msg)

		default:
		}
	})

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}

	detector.Start()

	go func() {
		time.Sleep(3 * time.Second)
		if node.Role == types.RoleFollower {
			if err := recovery.RequestRecovery(); err != nil {
				log.Println("recovery request error:", err)
			}
		}
	}()

	log.Printf("===== %s START =====", strings.ToUpper(node.ID))
	log.Printf("node %s started on %s as %s\n", node.ID, address, node.Role)
	log.Printf("node %s writing logs to log/%s.log\n", node.ID, node.ID)

	select {}
}
