package integration_test

// import (
// 	"fmt"
// 	"testing"
// 	"time"

// 	"distributed_payment_system/internal/clock"
// )

// func TestNTPSync(t *testing.T) {
// 	c1 := &clock.Clock{}
// 	c2 := &clock.Clock{}

// 	// simulate periodic sync
// 	c1.Sync()
// 	c2.Sync()

// 	diff := c1.Now().Sub(c2.Now())
// 	if diff < 0 {
// 		diff = -diff
// 	}

// 	fmt.Println("Clock difference:", diff)

// 	if diff > 50*time.Millisecond {
// 		t.Error("Clock sync difference exceeds 50ms")
// 	}
// }

// func TestHLCOrdering(t *testing.T) {
// 	c := &clock.Clock{}
// 	hlc := clock.NewHLC(c)

// 	local1 := hlc.Now()
// 	time.Sleep(1 * time.Millisecond)
// 	local2 := hlc.Now()

// 	if !(local2.Physical >= local1.Physical) {
// 		t.Error("HLC physical time did not advance")
// 	}

// 	// simulate remote timestamp in the past
// 	remote := clock.Timestamp{
// 		Physical: local1.Physical - 10,
// 		Logical:  0,
// 	}

// 	merged := hlc.Update(remote)
// 	if merged.Physical != local2.Physical {
// 		t.Log("HLC merged correctly with older remote timestamp")
// 	}
// }

// func TestClockSkew(t *testing.T) {
// 	local := time.Now()
// 	remote := local.Add(150 * time.Millisecond) // simulate skew

// 	isSkewed, diff := clock.DetectSkew(local, remote)
// 	if !isSkewed {
// 		t.Error("Skew detection failed")
// 	}
// 	fmt.Println("Detected skew:", diff)
// }

// func TestBufferOrdering(t *testing.T) {
// 	buffer := clock.NewBuffer()

// 	// Add logs with explicit out-of-order timestamps.
// 	base := time.Now().UnixNano()
// 	buffer.Add(clock.LogEntry{Timestamp: clock.Timestamp{Physical: base + 1_000_000, Logical: 0}, Data: "Local Event"})
// 	buffer.Add(clock.LogEntry{Timestamp: clock.Timestamp{Physical: base, Logical: 0}, Data: "Remote Event"})

// 	ordered := buffer.Flush()

// 	if ordered[0].Data != "Remote Event" {
// 		t.Error("Buffer did not reorder logs correctly")
// 	}
// }

// func TestFullPipeline(t *testing.T) {
// 	c1 := &clock.Clock{}
// 	c2 := &clock.Clock{}
// 	hlc1 := clock.NewHLC(c1)
// 	hlc2 := clock.NewHLC(c2)
// 	buffer := clock.NewBuffer()

// 	// simulate local and remote events
// 	localEvent := hlc1.Now()
// 	remoteEvent := hlc2.Now()

// 	buffer.Add(clock.LogEntry{Timestamp: localEvent, Data: "Local Payment"})
// 	buffer.Add(clock.LogEntry{Timestamp: hlc1.Update(remoteEvent), Data: "Remote Payment"})

// 	ordered := buffer.Flush()
// 	for i, log := range ordered {
// 		fmt.Printf("Event %d: %s | Physical=%d Logical=%d\n", i+1, log.Data, log.Timestamp.Physical, log.Timestamp.Logical)
// 	}
// }
