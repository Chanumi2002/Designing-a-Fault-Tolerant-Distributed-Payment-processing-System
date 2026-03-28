package main

import (
	"log"
	"time"

	"distributed_payment_system/internal/clock"
)

func main() {
	// Create clocks for 3 nodes
	clock1 := clock.NewClock()
	clock2 := clock.NewClock()
	clock3 := clock.NewClock()

	// Simulate node skew
	clock1.SetOffset(0 * time.Millisecond)
	clock2.SetOffset(50 * time.Millisecond)
	clock3.SetOffset(-30 * time.Millisecond)

	// Create HLC instances
	hlc1 := clock.NewHLC(clock1)
	hlc2 := clock.NewHLC(clock2)
	hlc3 := clock.NewHLC(clock3)

	log.Println("===== MEMBER 3 TEST START =====")

	// Show local clock times
	log.Printf("Node1 local time: %s\n", clock1.Now().Format(time.RFC3339Nano))
	log.Printf("Node2 local time: %s\n", clock2.Now().Format(time.RFC3339Nano))
	log.Printf("Node3 local time: %s\n", clock3.Now().Format(time.RFC3339Nano))

	// Skew detection
	log.Println("===== SKEW CHECK =====")
	clock.CheckNodeSkew("node2 vs node1", clock1.Now(), clock2.Now())
	clock.CheckNodeSkew("node3 vs node1", clock1.Now(), clock3.Now())

	// Generate local HLC timestamps
	log.Println("===== LOCAL HLC EVENTS =====")
	ts1 := hlc1.Now()
	log.Printf("Node1 HLC -> Physical=%d Logical=%d\n", ts1.Physical, ts1.Logical)

	ts2 := hlc2.Now()
	log.Printf("Node2 HLC -> Physical=%d Logical=%d\n", ts2.Physical, ts2.Logical)

	ts3 := hlc3.Now()
	log.Printf("Node3 HLC -> Physical=%d Logical=%d\n", ts3.Physical, ts3.Logical)

	// Merge remote timestamps
	log.Println("===== HLC MERGE TEST =====")
	mergedAtNode2 := hlc2.Update(ts1)
	log.Printf("Node2 merged Node1 timestamp -> Physical=%d Logical=%d\n", mergedAtNode2.Physical, mergedAtNode2.Logical)

	mergedAtNode3 := hlc3.Update(ts2)
	log.Printf("Node3 merged Node2 timestamp -> Physical=%d Logical=%d\n", mergedAtNode3.Physical, mergedAtNode3.Logical)

	// Reorder buffer test
	log.Println("===== REORDER BUFFER TEST =====")
	buffer := clock.NewBuffer()

	// Add deliberately out-of-order logs
	buffer.Add(clock.LogEntry{
		NodeID:      "node2",
		Event:       "replicated payment",
		Transaction: "txn3001",
		Timestamp:   ts2,
		ReceivedAt:  time.Now(),
	})

	buffer.Add(clock.LogEntry{
		NodeID:      "node1",
		Event:       "created payment",
		Transaction: "txn3001",
		Timestamp:   ts1,
		ReceivedAt:  time.Now(),
	})

	buffer.Add(clock.LogEntry{
		NodeID:      "node3",
		Event:       "acknowledged payment",
		Transaction: "txn3001",
		Timestamp:   ts3,
		ReceivedAt:  time.Now(),
	})

	ordered := buffer.Flush()

	log.Println("===== ORDERED LOG OUTPUT =====")
	for i, entry := range ordered {
		log.Printf(
			"%d. Node=%s Event=%s Txn=%s Physical=%d Logical=%d\n",
			i+1,
			entry.NodeID,
			entry.Event,
			entry.Transaction,
			entry.Timestamp.Physical,
			entry.Timestamp.Logical,
		)
	}

	log.Println("===== MEMBER 3 TEST END =====")
}
