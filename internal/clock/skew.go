package clock

import (
	"log"
	"math"
	"time"
)

// SkewThreshold defines the maximum acceptable clock difference.
const SkewThreshold = 100 * time.Millisecond

// DetectSkew compares local and remote times and returns:
// - whether the skew is too high
// - the absolute difference
func DetectSkew(local time.Time, remote time.Time) (bool, time.Duration) {
	diff := local.Sub(remote)

	if diff < 0 {
		diff = time.Duration(math.Abs(float64(diff)))
	}

	if diff > SkewThreshold {
		return true, diff
	}

	return false, diff
}

// CheckNodeSkew logs skew information for one node comparison.
func CheckNodeSkew(nodeID string, local time.Time, remote time.Time) {
	isSkewed, diff := DetectSkew(local, remote)

	if isSkewed {
		log.Printf("Node %s clock skew detected: %v\n", nodeID, diff)
	} else {
		log.Printf("Node %s clock within range: %v\n", nodeID, diff)
	}
}

// ExplainSkewCorrection prints simple correction ideas.
func ExplainSkewCorrection() {
	log.Println("Clock skew can be reduced using:")
	log.Println("- NTP synchronization")
	log.Println("- Hybrid Logical Clocks for ordering")
	log.Println("- Threshold-based skew detection")
	log.Println("- Reordering delayed logs before processing")
}