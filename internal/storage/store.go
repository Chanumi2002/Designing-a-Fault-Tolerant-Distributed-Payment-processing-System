package storage

import "distributed_payment_system/internal/types"

// Store defines the payment storage behavior used by replication logic.
type Store interface {
	SavePayment(payment *types.PaymentEntry) error
	GetPayment(transactionID string) (*types.PaymentEntry, bool)
	GetAllPayments() []*types.PaymentEntry
	HasPayment(transactionID string) bool
}