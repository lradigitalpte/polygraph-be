package appointments

import (
	"time"

	"gorm.io/gorm"
	"my-app/internal/money"
)

// QuotationEffectivePayments returns itemized payments for reporting.
// Uses payment_history when present; otherwise synthesizes one line from collected_amount
// (common for booking-linked invoices synced from appointments without history rows).
func QuotationEffectivePayments(db *gorm.DB, q *Quotation) []QuotationPaymentEntry {
	if q == nil {
		return nil
	}
	entries := parseQuotationPaymentHistory(q.PaymentHistoryJSON)
	if len(entries) > 0 {
		return entries
	}
	collected := money.CeilWhole(q.CollectedAmount)
	if collected <= 0 {
		return nil
	}
	paidAt := q.UpdatedAt.UTC()
	if paidAt.IsZero() {
		paidAt = q.CreatedAt.UTC()
	}
	if db != nil && q.AppointmentID != nil {
		var appt Appointment
		if err := db.Select("updated_at").First(&appt, *q.AppointmentID).Error; err == nil {
			if t := appt.UpdatedAt.UTC(); !t.IsZero() {
				paidAt = t
			}
		}
	}
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	return []QuotationPaymentEntry{
		{
			PaidAt:       paidAt,
			Amount:       collected,
			TotalCharged: collected,
			Method:       "recorded",
		},
	}
}
