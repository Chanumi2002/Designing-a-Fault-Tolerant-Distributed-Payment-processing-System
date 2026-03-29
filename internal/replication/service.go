package replication

import (
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/storage"
	"distributed_payment_system/internal/types"
)

var ErrDuplicatePayment = errors.New("duplicate payment transaction")

type PeerSender interface {
	Send(addr string, data []byte) error
}

type Service struct {
	store  storage.Store
	node   *types.Node
	config *config.Config
	sender PeerSender

	ackMu sync.Mutex
	acks  map[string]int
}

func NewService(store storage.Store, node *types.Node, cfg *config.Config, sender PeerSender) *Service {
	return &Service{
		store:  store,
		node:   node,
		config: cfg,
		sender: sender,
		acks:   make(map[string]int),
	}
}

func (s *Service) CreatePayment(transactionID string, amount float64, ownerID string) (*types.PaymentEntry, error) {
	if s.store.HasPayment(transactionID) {
		return nil, ErrDuplicatePayment
	}

	payment := &types.PaymentEntry{
		TransactionID: transactionID,
		Amount:        amount,
		Currency:      "USD",
		Status:        "pending",
		Timestamp:     float64(time.Now().UnixNano()) / 1e9,
		Version:       1,
		OwnerID:       ownerID,
		StripeID:      "",
	}

	if err := s.store.SavePayment(payment); err != nil {
		return nil, err
	}

	s.ackMu.Lock()
	s.acks[transactionID] = 1
	s.ackMu.Unlock()

	if err := s.ReplicatePaymentToFollowers(payment); err != nil {
		return nil, err
	}

	return payment, nil
}

func (s *Service) ReplicatePaymentToFollowers(payment *types.PaymentEntry) error {
	peers := s.config.PeersExcluding(s.node.ID)

	for _, peer := range peers {
		payload := map[string]interface{}{
			"transaction_id": payment.TransactionID,
			"amount":         payment.Amount,
			"currency":       payment.Currency,
			"status":         payment.Status,
			"timestamp":      payment.Timestamp,
			"version":        payment.Version,
			"owner_id":       payment.OwnerID,
			"stripe_id":      payment.StripeID,
		}

		data, err := types.NewMessage(types.MsgPaymentReplicate, s.node.ID, payload)
		if err != nil {
			continue
		}

		addr := net.JoinHostPort(peer.Host, strconv.Itoa(peer.Port))
		_ = s.sender.Send(addr, data)
	}

	return nil
}

func (s *Service) HandleReplicatedPayment(msg *types.Message) error {
	if msg == nil {
		return nil
	}

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
		return nil
	}

	if !s.store.HasPayment(transactionID) {
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

		if err := s.store.SavePayment(payment); err != nil {
			return err
		}
	}

	payload := map[string]interface{}{
		"transaction_id": transactionID,
	}

	data, err := types.NewMessage(types.MsgPaymentAck, s.node.ID, payload)
	if err != nil {
		return err
	}

	leaderAddr := s.nodeAddrFromID(msg.Sender)
	if leaderAddr == "" {
		return nil
	}

	return s.sender.Send(leaderAddr, data)
}

func (s *Service) HandlePaymentAck(msg *types.Message) {
	if msg == nil {
		return
	}

	transactionID, _ := msg.Payload["transaction_id"].(string)
	if transactionID == "" {
		return
	}

	s.ackMu.Lock()
	defer s.ackMu.Unlock()

	s.acks[transactionID]++
}

func (s *Service) GetAckCount(transactionID string) int {
	s.ackMu.Lock()
	defer s.ackMu.Unlock()

	return s.acks[transactionID]
}

func (s *Service) HasMajority(transactionID string) bool {
	s.ackMu.Lock()
	defer s.ackMu.Unlock()

	required := (len(s.config.Nodes) / 2) + 1
	return s.acks[transactionID] >= required
}

func (s *Service) MarkCommitted(transactionID string) {
	payment, ok := s.store.GetPayment(transactionID)
	if !ok {
		return
	}

	payment.Status = "committed"
	_ = s.store.SavePayment(payment)

	log.Printf(
		"COMMITTED | node=%s | txn=%s | amount=%.2f | owner=%s | status=%s | version=%d",
		s.node.ID,
		payment.TransactionID,
		payment.Amount,
		payment.OwnerID,
		payment.Status,
		payment.Version,
	)
}

func (s *Service) BroadcastCommit(transactionID string) error {
	payment, ok := s.store.GetPayment(transactionID)
	if !ok {
		return nil
	}

	peers := s.config.PeersExcluding(s.node.ID)

	for _, peer := range peers {
		payload := map[string]interface{}{
			"transaction_id": payment.TransactionID,
			"amount":         payment.Amount,
			"currency":       payment.Currency,
			"status":         "committed",
			"timestamp":      payment.Timestamp,
			"version":        payment.Version,
			"owner_id":       payment.OwnerID,
			"stripe_id":      payment.StripeID,
		}

		data, err := types.NewMessage(types.MsgPaymentCommit, s.node.ID, payload)
		if err != nil {
			continue
		}

		addr := net.JoinHostPort(peer.Host, strconv.Itoa(peer.Port))
		_ = s.sender.Send(addr, data)
	}

	return nil
}

func (s *Service) HandleCommitPayment(msg *types.Message) error {
	if msg == nil {
		return nil
	}

	transactionID, _ := msg.Payload["transaction_id"].(string)
	if transactionID == "" {
		return nil
	}

	payment, ok := s.store.GetPayment(transactionID)
	if !ok {
		currency, _ := msg.Payload["currency"].(string)
		status, _ := msg.Payload["status"].(string)
		ownerID, _ := msg.Payload["owner_id"].(string)
		stripeID, _ := msg.Payload["stripe_id"].(string)
		amount, _ := msg.Payload["amount"].(float64)
		timestamp, _ := msg.Payload["timestamp"].(float64)
		versionFloat, _ := msg.Payload["version"].(float64)
		version := int(versionFloat)

		payment = &types.PaymentEntry{
			TransactionID: transactionID,
			Amount:        amount,
			Currency:      currency,
			Status:        status,
			Timestamp:     timestamp,
			Version:       version,
			OwnerID:       ownerID,
			StripeID:      stripeID,
		}
	} else {
		payment.Status = "committed"
	}

	if err := s.store.SavePayment(payment); err != nil {
		return err
	}

	log.Printf(
		"COMMITTED | node=%s | txn=%s | amount=%.2f | owner=%s | status=%s | version=%d",
		s.node.ID,
		payment.TransactionID,
		payment.Amount,
		payment.OwnerID,
		payment.Status,
		payment.Version,
	)

	return nil
}

func (s *Service) GetPayment(transactionID string) (*types.PaymentEntry, bool) {
	return s.store.GetPayment(transactionID)
}

func (s *Service) GetAllPayments() []*types.PaymentEntry {
	return s.store.GetAllPayments()
}

func (s *Service) ApplyRecoveredPayment(payment *types.PaymentEntry) error {
	if payment == nil || payment.TransactionID == "" {
		return nil
	}

	if s.store.HasPayment(payment.TransactionID) {
		return nil
	}

	return s.store.SavePayment(payment)
}

func (s *Service) nodeAddrFromID(nodeID string) string {
	for _, n := range s.config.Nodes {
		if n.ID == nodeID {
			return net.JoinHostPort(n.Host, strconv.Itoa(n.Port))
		}
	}
	return ""
}
