package fault

import (
	"log"
	"net"
	"strconv"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/types"
)

// RecoveryManager handles node rejoin and data synchronization.
// When a failed node comes back, it asks the leader for the
// latest payment data so it can catch up.
type RecoveryManager struct {
	Node   *types.Node
	Config *config.Config
	Sender PeerSender
}

// NewRecoveryManager creates a new recovery manager instance.
func NewRecoveryManager(node *types.Node, cfg *config.Config, sender PeerSender) *RecoveryManager {
	return &RecoveryManager{
		Node:   node,
		Config: cfg,
		Sender: sender,
	}
}

// RequestRecovery is called by a recovering follower node.
// It sends a recovery request to the currently known leader.
func (r *RecoveryManager) RequestRecovery() error {
	r.Node.Mu.RLock()
	leaderID := r.Node.KnownLeader
	r.Node.Mu.RUnlock()

	// If no leader is known yet, recovery cannot proceed.
	if leaderID == "" {
		log.Println("recovery skipped: no known leader")
		return nil
	}

	leaderCfg, ok := r.Config.FindNode(leaderID)
	if !ok {
		log.Println("recovery skipped: leader not found in config")
		return nil
	}

	// For this simple version, we just send how many payments
	// this node currently has. Later this can be improved to
	// send log index / version information.
	r.Node.Mu.RLock()
	currentCount := len(r.Node.Payments)
	r.Node.Mu.RUnlock()

	payload := map[string]interface{}{
		"node_id":       r.Node.ID,
		"payment_count": currentCount,
	}

	data, err := types.NewMessage(types.MsgRecoveryRequest, r.Node.ID, payload)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(leaderCfg.Host, strconv.Itoa(leaderCfg.Port))
	log.Printf("requesting recovery from leader %s at %s\n", leaderID, addr)

	return r.Sender.Send(addr, data)
}

// HandleRecoveryRequest is called on the leader side.
// It receives a recovery request from a follower and sends
// payment data back to that recovering node.
func (r *RecoveryManager) HandleRecoveryRequest(msg *types.Message) error {
	if msg == nil {
		return nil
	}

	targetNodeID, ok := msg.Payload["node_id"].(string)
	if !ok || targetNodeID == "" {
		log.Println("invalid recovery request: missing node_id")
		return nil
	}

	targetCfg, ok := r.Config.FindNode(targetNodeID)
	if !ok {
		log.Printf("recovery target %s not found in config\n", targetNodeID)
		return nil
	}

	// For now, send all payments known by the leader.
	// Later this can be optimized to send only missing entries.
	r.Node.Mu.RLock()
	payments := make([]map[string]interface{}, 0, len(r.Node.Payments))
	for _, p := range r.Node.Payments {
		payments = append(payments, map[string]interface{}{
			"transaction_id": p.TransactionID,
			"amount":         p.Amount,
			"currency":       p.Currency,
			"status":         p.Status,
			"timestamp":      p.Timestamp,
			"version":        p.Version,
			"owner_id":       p.OwnerID,
			"stripe_id":      p.StripeID,
		})
	}
	r.Node.Mu.RUnlock()

	payload := map[string]interface{}{
		"payments": payments,
	}

	data, err := types.NewMessage(types.MsgRecoveryData, r.Node.ID, payload)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(targetCfg.Host, strconv.Itoa(targetCfg.Port))
	log.Printf("sending recovery data to %s at %s\n", targetNodeID, addr)

	return r.Sender.Send(addr, data)
}

// HandleRecoveryData is called on the recovering node.
// It receives payment data from the leader and applies it locally.
func (r *RecoveryManager) HandleRecoveryData(msg *types.Message) {
	if msg == nil {
		return
	}

	rawPayments, ok := msg.Payload["payments"].([]interface{})
	if !ok {
		log.Println("invalid recovery payload: payments missing")
		return
	}

	r.Node.Mu.Lock()
	defer r.Node.Mu.Unlock()

	for _, item := range rawPayments {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		transactionID, _ := record["transaction_id"].(string)
		currency, _ := record["currency"].(string)
		status, _ := record["status"].(string)
		ownerID, _ := record["owner_id"].(string)
		stripeID, _ := record["stripe_id"].(string)

		amount, _ := record["amount"].(float64)
		timestamp, _ := record["timestamp"].(float64)

		versionFloat, _ := record["version"].(float64)
		version := int(versionFloat)

		// Ignore invalid payment records
		if transactionID == "" {
			continue
		}

		r.Node.Payments[transactionID] = &types.PaymentEntry{
			TransactionID: transactionID,
			Amount:        amount,
			Currency:      currency,
			Status:        status,
			Timestamp:     timestamp,
			Version:       version,
			OwnerID:       ownerID,
			StripeID:      stripeID,
		}
	}

	log.Printf("recovery applied successfully on node %s\n", r.Node.ID)
	log.Printf("node %s now has %d payments after recovery\n", r.Node.ID, len(r.Node.Payments))
}
