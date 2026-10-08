package appointments

import (
	"strings"
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
		out := make([]QuotationPaymentEntry, len(entries))
		for i, entry := range entries {
			entry.PaidAt = quotationReportPaymentDate(db, q, entry)
			out[i] = entry
		}
		return out
	}
	collected := money.CeilWhole(q.CollectedAmount)
	if collected <= 0 {
		return nil
	}
	return []QuotationPaymentEntry{
		{
			PaidAt:       quotationSyntheticPaymentDate(db, q),
			Amount:       collected,
			TotalCharged: collected,
			Method:       "recorded",
		},
	}
}

// quotationReportPaymentDate picks a reporting date. Stripe/manual entries keep paid_at;
// legacy booking sync rows use exam (scheduled) date when available instead of updated_at.
func quotationReportPaymentDate(db *gorm.DB, q *Quotation, entry QuotationPaymentEntry) time.Time {
	method := strings.ToLower(strings.TrimSpace(entry.Method))
	if method != "recorded" && method != "appointment" {
		if t := entry.PaidAt.UTC(); !t.IsZero() {
			return t
		}
		return quotationSyntheticPaymentDate(db, q)
	}
	return quotationSyntheticPaymentDate(db, q)
}

func quotationSyntheticPaymentDate(db *gorm.DB, q *Quotation) time.Time {
	if db != nil && q.AppointmentID != nil {
		var appt Appointment
		if err := db.Select("scheduled_at").First(&appt, *q.AppointmentID).Error; err == nil {
			if t := appt.ScheduledAt.UTC(); !t.IsZero() {
				return t
			}
		}
	}
	if t := q.CreatedAt.UTC(); !t.IsZero() {
		return t
	}
	if t := q.UpdatedAt.UTC(); !t.IsZero() {
		return t
	}
	return time.Now().UTC()
}
