package fault

import (
	"log"
	"net"
	"strconv"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/types"
)

type RecoveryManager struct {
	Node   *types.Node
	Config *config.Config
	Sender PeerSender
}

func NewRecoveryManager(node *types.Node, cfg *config.Config, sender PeerSender) *RecoveryManager {
	return &RecoveryManager{
		Node:   node,
		Config: cfg,
		Sender: sender,
	}
}

func (r *RecoveryManager) RequestRecovery() error {
	r.Node.Mu.RLock()
	if !r.Node.IsActive || r.Node.Status == types.StatusFailed {
		r.Node.Mu.RUnlock()
		return nil
	}
	leaderID := r.Node.KnownLeader
	currentCount := len(r.Node.Payments)
	r.Node.Mu.RUnlock()

	if leaderID == "" {
		return nil
	}

	leaderCfg, ok := r.Config.FindNode(leaderID)
	if !ok {
		return nil
	}

	payload := map[string]interface{}{
		"node_id":       r.Node.ID,
		"payment_count": currentCount,
	}

	data, err := types.NewMessage(types.MsgRecoveryRequest, r.Node.ID, payload)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(leaderCfg.Host, strconv.Itoa(leaderCfg.Port))
	return r.Sender.Send(addr, data)
}

func (r *RecoveryManager) HandleRecoveryRequest(msg *types.Message) error {
	if msg == nil {
		return nil
	}

	r.Node.Mu.RLock()
	if !r.Node.IsActive || r.Node.Status == types.StatusFailed {
		r.Node.Mu.RUnlock()
		return nil
	}
	r.Node.Mu.RUnlock()

	targetNodeID, ok := msg.Payload["node_id"].(string)
	if !ok || targetNodeID == "" {
		return nil
	}

	targetCfg, ok := r.Config.FindNode(targetNodeID)
	if !ok {
		return nil
	}

	r.Node.Mu.RLock()
	payments := make([]map[string]interface{}, 0, len(r.Node.Payments))
	for _, p := range r.Node.Payments {
		if p == nil || p.Status != "committed" {
			continue
		}

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
	return r.Sender.Send(addr, data)
}

func (r *RecoveryManager) HandleRecoveryData(msg *types.Message) {
	if msg == nil {
		return
	}

	r.Node.Mu.RLock()
	if !r.Node.IsActive || r.Node.Status == types.StatusFailed {
		r.Node.Mu.RUnlock()
		return
	}
	r.Node.Mu.RUnlock()

	rawPayments, ok := msg.Payload["payments"].([]interface{})
	if !ok {
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

		if transactionID == "" || status != "committed" {
			continue
		}

		existing, exists := r.Node.Payments[transactionID]
		shouldLog := false

		if !exists {
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
			shouldLog = true
		} else if existing.Status != "committed" {
			existing.Amount = amount
			existing.Currency = currency
			existing.Status = "committed"
			existing.Timestamp = timestamp
			existing.Version = version
			existing.OwnerID = ownerID
			existing.StripeID = stripeID
			shouldLog = true
		}

		if shouldLog {
			log.Printf(
				"txn=%s | amount=%.2f | owner=%s",
				transactionID,
				amount,
				ownerID,
			)
		}
	}
}
