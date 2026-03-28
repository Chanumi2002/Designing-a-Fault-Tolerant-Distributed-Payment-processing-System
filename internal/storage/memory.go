package storage

import (
	"sync"

	"distributed_payment_system/internal/types"
)

// MemoryStore is a simple in-memory payment ledger.
// Good enough for first testing before adding disk persistence.
type MemoryStore struct {
	mu       sync.RWMutex
	payments map[string]*types.PaymentEntry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		payments: make(map[string]*types.PaymentEntry),
	}
}

// SavePayment stores a payment using TransactionID as the unique key.
func (s *MemoryStore) SavePayment(payment *types.PaymentEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.payments[payment.TransactionID] = payment
	return nil
}

// GetPayment returns a payment by transaction ID.
func (s *MemoryStore) GetPayment(transactionID string) (*types.PaymentEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.payments[transactionID]
	return p, ok
}

// GetAllPayments returns all stored payments.
func (s *MemoryStore) GetAllPayments() []*types.PaymentEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*types.PaymentEntry, 0, len(s.payments))
	for _, p := range s.payments {
		result = append(result, p)
	}
	return result
}

// HasPayment checks whether the transaction already exists.
// This is the first deduplication step.
func (s *MemoryStore) HasPayment(transactionID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.payments[transactionID]
	return exists
}