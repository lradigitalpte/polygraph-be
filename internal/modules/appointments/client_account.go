package appointments

import "time"

// AccountSummary is rolled-up billing for a client.
type AccountSummary struct {
	TotalBilled float64 `json:"total_billed"`
	TotalPaid   float64 `json:"total_paid"`
	BalanceDue  float64 `json:"balance_due"`
}

// AccountLedgerEntry is one billable line (unified across Payments and client account).
type AccountLedgerEntry struct {
	ID            uint      `json:"id"`
	Source        string    `json:"source"` // booking | session | quote
	Code          string    `json:"code"`
	ReferenceID   uint      `json:"reference_id"`
	AppointmentID *uint     `json:"appointment_id,omitempty"`
	QuotationID   *uint     `json:"quotation_id,omitempty"`
	ClientID      uint      `json:"client_id"`
	ClientName    string    `json:"client_name,omitempty"`
	ClientEmail   string    `json:"client_email,omitempty"`
	Title         string    `json:"title"`
	Date          time.Time `json:"date"`
	TotalAmount   float64   `json:"total_amount"`
	PaidAmount    float64   `json:"paid_amount"`
	BalanceDue    float64   `json:"balance_due"`
	Status        string    `json:"status"`
	PaymentMode   string    `json:"payment_mode,omitempty"`
	Currency      string    `json:"currency,omitempty"`
	ExaminerName  string    `json:"examiner_name,omitempty"`
}
