package main

import (
	"fmt"
	"log"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/replication"
	"distributed_payment_system/internal/storage"
	"distributed_payment_system/internal/types"
)

type dummySender struct{}

func (d *dummySender) Send(addr string, data []byte) error {
	return nil
}

func main() {
	node := types.NewNode("node1", "127.0.0.1", 8001)
	cfg := config.Load()
	store := storage.NewMemoryStore()
	sender := &dummySender{}
	service := replication.NewService(store, node, cfg, sender)

	fmt.Println("TEST 1: create txn1001 first time")
	p1, err := service.CreatePayment("txn1001", 100.50, "user1")
	if err != nil {
		log.Println("failed:", err)
	} else {
		log.Printf("success: %+v\n", p1)
	}

	fmt.Println("TEST 2: create txn1001 second time")
	p2, err := service.CreatePayment("txn1001", 100.50, "user1")
	if err != nil {
		log.Println("duplicate detected correctly:", err)
	} else {
		log.Printf("unexpected success: %+v\n", p2)
	}

	fmt.Println("TEST 3: create txn1002")
	p3, err := service.CreatePayment("txn1002", 250.00, "user2")
	if err != nil {
		log.Println("failed:", err)
	} else {
		log.Printf("success: %+v\n", p3)
	}

	fmt.Println("TEST 4: print all payments")
	all := service.GetAllPayments()
	for _, p := range all {
		log.Printf("payment: %+v\n", p)
	}
}
