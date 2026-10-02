package appointments

import (
	"encoding/json"
	"strings"
	"time"
)

// QuotationPaymentEntry is one payment applied to an invoice (Stripe or manual).
type QuotationPaymentEntry struct {
	PaidAt          time.Time `json:"paid_at"`
	Amount          float64   `json:"amount"`
	ProcessingFee   float64   `json:"processing_fee,omitempty"`
	TotalCharged    float64   `json:"total_charged,omitempty"`
	Method          string    `json:"method"`
	StripeSessionID string    `json:"stripe_session_id,omitempty"`
}

func parseQuotationPaymentHistory(raw string) []QuotationPaymentEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var entries []QuotationPaymentEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	return entries
}

func encodeQuotationPaymentHistory(entries []QuotationPaymentEntry) string {
	if len(entries) == 0 {
		return "[]"
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func appendQuotationPaymentEntry(raw string, entry QuotationPaymentEntry) string {
	entries := parseQuotationPaymentHistory(raw)
	if entry.PaidAt.IsZero() {
		entry.PaidAt = time.Now().UTC()
	}
	if strings.TrimSpace(entry.Method) == "" {
		entry.Method = "manual"
	}
	if entry.TotalCharged <= 0 {
		entry.TotalCharged = entry.Amount + entry.ProcessingFee
	}
	entries = append(entries, entry)
	return encodeQuotationPaymentHistory(entries)
}
