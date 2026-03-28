package types

type PaymentEntry struct {
	TransactionID string
	Amount        float64
	Currency      string
	Status        string
	Timestamp     float64
	Version       int
	OwnerID       string
	StripeID      string
}

func NewPaymentEntry(txnID string, amount float64, ownerID string) *PaymentEntry {
	return &PaymentEntry{
		TransactionID: txnID,
		Amount:        amount,
		Currency:      "USD",
		Status:        "pending",
		Version:       1,
		OwnerID:       ownerID,
	}
}
