package appointments

// Quote-first examination packages for corporate clients.
//
// A standalone invoice for a corporate / law-firm client is a bulk package: it creates
// one examination slot per line_quantity as soon as it is created, before any payment.
// Bookings use those slots and do NOT get their own session invoice; the bulk invoice is
// the only bill. Each slot-backed booking shows Paid/Unpaid to match its bulk invoice.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"my-app/internal/money"
)

// isCorporateBulkQuote reports whether a standalone invoice belongs to a corporate
// client, i.e. it is an examination package that creates slots when created.
func isCorporateBulkQuote(tx *gorm.DB, quote *Quotation) bool {
	if quote == nil || quote.AppointmentID != nil || quote.ClientID == 0 {
		return false
	}
	var client Client
	if err := tx.Select("id", "client_type").First(&client, quote.ClientID).Error; err != nil {
		return false
	}
	return isCorporateClientType(client.ClientType)
}

// reconcileSlotCountForQuotation keeps a bulk invoice's slot count equal to its
// line_quantity after it is edited: adds slots, or removes unused ones. It refuses to go
// below the number of slots already used by bookings.
func (s *Service) reconcileSlotCountForQuotation(tx *gorm.DB, quote *Quotation) error {
	if quote == nil || quote.ID == 0 || quote.AppointmentID != nil {
		return nil
	}
	var total, consumed int64
	if err := tx.Model(&ExamPrepaidSlot{}).Where("source_quotation_id = ?", quote.ID).Count(&total).Error; err != nil {
		return err
	}
	if total == 0 {
		return nil // invoice never had slots; nothing to keep in step
	}
	if err := tx.Model(&ExamPrepaidSlot{}).
		Where("source_quotation_id = ? AND status = ?", quote.ID, "consumed").
		Count(&consumed).Error; err != nil {
		return err
	}
	qty := int64(slotQuantityForQuotation(quote))
	if qty < consumed {
		return fmt.Errorf("%d examinations on this invoice are already booked; quantity cannot be lower than that", consumed)
	}
	if qty > total {
		return s.ensureSlotsForQuotation(tx, quote)
	}
	if qty < total {
		var spare []ExamPrepaidSlot
		if err := tx.Where("source_quotation_id = ? AND status = ?", quote.ID, "available").
			Order("created_at DESC, id DESC").
			Limit(int(total - qty)).
			Find(&spare).Error; err != nil {
			return err
		}
		for i := range spare {
			if err := tx.Delete(&spare[i]).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func bulkQuoteCode(quote *Quotation) string {
	code := strings.TrimSpace(quote.Code)
	if code == "" {
		code = fmt.Sprintf("%d", quote.ID)
	}
	return code
}

// slotBookingPayment derives a slot-backed booking's payment fields from its bulk invoice.
func slotBookingPayment(quote *Quotation, examFee float64) (status string, collected float64, mode string) {
	mode = "Bulk invoice " + bulkQuoteCode(quote)
	if quotationFullyPaid(quote) {
		return "Paid", examFee, mode
	}
	// Deposit paid (e.g. 30% up front): the exam carries the same share of its fee.
	if total := QuotationTotalDue(quote); total > 0 && quote.CollectedAmount > 0 {
		share := quote.CollectedAmount / total
		return "Partial", money.Round2(examFee * share), mode
	}
	return "Unpaid", 0, mode
}

// releaseSlotForCancelledAppointment gives a cancelled booking's slot back to its bulk
// invoice so it can be booked again, and detaches the booking from the invoice.
func releaseSlotForCancelledAppointment(tx *gorm.DB, appointmentID uint) error {
	var appt Appointment
	if err := tx.First(&appt, appointmentID).Error; err != nil {
		return err
	}
	if appt.PrepaidSlotID == nil {
		return nil
	}
	if _, err := releasePrepaidSlotForAppointment(tx, &appt); err != nil {
		return err
	}
	// Older slot bookings also got a session invoice marked paid from the slot; drop it
	// (unless real money was recorded on it) so the cancelled exam is not billed.
	var legacy []Quotation
	if err := tx.Where("appointment_id = ?", appt.ID).Find(&legacy).Error; err != nil {
		return err
	}
	for i := range legacy {
		if !quotationHasRecordedPayment(&legacy[i]) {
			if err := tx.Delete(&legacy[i]).Error; err != nil {
				return err
			}
		}
	}
	return tx.Model(&Appointment{}).Where("id = ?", appt.ID).Updates(map[string]interface{}{
		"prepaid_slot_id":  nil,
		"payment_status":   "Unpaid",
		"collected_amount": 0,
		"payment_mode":     "",
	}).Error
}

// releaseSlotOnCancelUnlessCharged returns a cancelled exam's slot, except for clients
// whose agreement counts a cancellation as used.
func releaseSlotOnCancelUnlessCharged(tx *gorm.DB, appointmentID uint) error {
	var appt Appointment
	if err := tx.Select("id", "client_id", "prepaid_slot_id").First(&appt, appointmentID).Error; err != nil {
		return err
	}
	if appt.PrepaidSlotID == nil {
		return nil
	}
	var client Client
	if err := tx.Select("id", "cancellation_counts_as_used").First(&client, appt.ClientID).Error; err == nil &&
		client.CancellationCountsAsUsed {
		return nil
	}
	return releaseSlotForCancelledAppointment(tx, appt.ID)
}

// SetCancellationCountsAsUsed sets a client's cancellation policy for bulk-invoice slots.
func (s *Service) SetCancellationCountsAsUsed(clientID string, value bool) (*PrepaidSlotSummary, error) {
	if _, err := s.GetClientByID(clientID); err != nil {
		return nil, err
	}
	if err := s.db.Model(&Client{}).Where("id = ?", clientID).
		Update("cancellation_counts_as_used", value).Error; err != nil {
		return nil, err
	}
	return s.GetClientPrepaidSlotSummary(clientID)
}

// quotationHasRecordedPayment reports real money (card or manual entry) on an invoice,
// as opposed to an amount that was only marked paid at booking time.
func quotationHasRecordedPayment(q *Quotation) bool {
	for _, p := range parseQuotationPaymentHistory(q.PaymentHistoryJSON) {
		m := strings.ToLower(strings.TrimSpace(p.Method))
		if m == "stripe" || m == "manual" {
			return true
		}
	}
	return false
}

func isCancelledStatus(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "cancelled" || s == "canceled"
}

// RevokeSlotFromQuotation is a correction: it removes one unused examination slot from
// a bulk invoice, lowers its exam count by one and reprices it (same unit price, same
// discount and VAT rate). Refused when no slot is free, when it is the last exam, or
// when the new total would be below what was already paid (refund first).
func (s *Service) RevokeSlotFromQuotation(quotationID string) (*Quotation, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var quote Quotation
		if err := tx.First(&quote, quotationID).Error; err != nil {
			return errors.New("quotation not found")
		}
		if quote.AppointmentID != nil {
			return errors.New("only bulk invoices have examination slots")
		}
		qty := slotQuantityForQuotation(&quote)
		if qty <= 1 {
			return errors.New("this is the last examination on the invoice; delete the invoice instead")
		}
		var slot ExamPrepaidSlot
		if err := tx.Where("source_quotation_id = ? AND status = ?", quote.ID, "available").
			Order("created_at DESC, id DESC").First(&slot).Error; err != nil {
			return errors.New("no unused slot to revoke; cancel one of the booked exams first")
		}

		subtotal := quote.SubtotalAmount
		if subtotal <= 0 {
			subtotal = quote.Amount - quote.VatAmount
		}
		unit := subtotal / float64(qty)
		next := quote
		next.LineQuantity = qty - 1
		next.SubtotalAmount = money.CeilWhole(subtotal - unit)
		totals := computeInvoiceTotals(&next)
		next.VatAmount = totals.VAT
		next.Amount = totals.Total
		if money.CeilWhole(quote.CollectedAmount) > money.CeilWhole(next.Amount) {
			return fmt.Errorf("already paid %.2f is more than the new total %.2f; refund the difference first",
				quote.CollectedAmount, next.Amount)
		}

		if err := tx.Delete(&slot).Error; err != nil {
			return err
		}
		if err := tx.Model(&Quotation{}).Where("id = ?", quote.ID).Updates(map[string]interface{}{
			"line_quantity":   next.LineQuantity,
			"subtotal_amount": next.SubtotalAmount,
			"vat_amount":      next.VatAmount,
			"amount":          next.Amount,
			"status":          quotationPaymentStatusFromAmounts(quote.CollectedAmount, next.Amount),
		}).Error; err != nil {
			return err
		}
		txSvc := &Service{db: tx, storage: s.storage}
		return txSvc.syncSlotBookingsFromBulkQuote(tx, quote.ID)
	})
	if err != nil {
		return nil, err
	}
	return s.GetQuotationByID(quotationID)
}

// syncSlotBookingsFromBulkQuote marks every booking that used a slot of this invoice as
// paid or unpaid to match the invoice. Call it whenever the invoice's payments change.
func (s *Service) syncSlotBookingsFromBulkQuote(tx *gorm.DB, quoteID uint) error {
	var quote Quotation
	if err := tx.First(&quote, quoteID).Error; err != nil {
		return err
	}
	if quote.AppointmentID != nil {
		return nil
	}
	var slots []ExamPrepaidSlot
	if err := tx.Where("source_quotation_id = ? AND status = ? AND appointment_id IS NOT NULL", quote.ID, "consumed").
		Find(&slots).Error; err != nil {
		return err
	}
	for _, slot := range slots {
		var appt Appointment
		if err := tx.First(&appt, *slot.AppointmentID).Error; err != nil {
			continue
		}
		status, collected, mode := slotBookingPayment(&quote, appt.ExamFee)
		if err := tx.Model(&Appointment{}).Where("id = ?", appt.ID).Updates(map[string]interface{}{
			"payment_status":   status,
			"collected_amount": collected,
			"payment_mode":     mode,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// guardBulkQuoteDelete stops deleting a bulk invoice whose slots are booked, and removes
// its unused slots when deletion is allowed.
func guardBulkQuoteDelete(tx *gorm.DB, quoteID uint) error {
	var consumed int64
	if err := tx.Model(&ExamPrepaidSlot{}).
		Where("source_quotation_id = ? AND status = ?", quoteID, "consumed").
		Count(&consumed).Error; err != nil {
		return err
	}
	if consumed > 0 {
		return fmt.Errorf("%d bookings use examination slots from this invoice; delete or move those bookings first", consumed)
	}
	return tx.Where("source_quotation_id = ? AND status = ?", quoteID, "available").Delete(&ExamPrepaidSlot{}).Error
}

// ApplyPrepaidSlotToAppointment moves an existing booking onto the client's next free
// slot: the booking is then billed through the bulk invoice and its own session invoice
// is removed. Fixes bookings made the normal way while a bulk invoice already covered them.
func (s *Service) ApplyPrepaidSlotToAppointment(appointmentID string) (*Appointment, error) {
	var out Appointment
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var appt Appointment
		if err := tx.First(&appt, appointmentID).Error; err != nil {
			return errors.New("appointment not found")
		}
		if appt.PrepaidSlotID != nil {
			return errors.New("this booking already uses a prepaid slot")
		}
		if strings.EqualFold(strings.TrimSpace(appt.Status), "cancelled") {
			return errors.New("cancelled bookings cannot use a prepaid slot")
		}

		var sessionQuotes []Quotation
		if err := tx.Where("appointment_id = ?", appt.ID).Find(&sessionQuotes).Error; err != nil {
			return err
		}
		for i := range sessionQuotes {
			if quotationHasRecordedPayment(&sessionQuotes[i]) {
				return fmt.Errorf("invoice %s has a recorded payment; remove that payment first", bulkQuoteCode(&sessionQuotes[i]))
			}
		}

		svc := &Service{db: tx, storage: s.storage}
		slot, note, err := svc.redeemPrepaidSlotForBooking(tx, appt.ClientID)
		if err != nil {
			return err
		}
		if slot == nil {
			return errors.New("this client has no free examination slots")
		}
		quote := slot.SourceQuotation
		if quote.ID == 0 {
			if err := tx.First(&quote, slot.SourceQuotationID).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&ExamPrepaidSlot{}).Where("id = ?", slot.ID).Updates(map[string]interface{}{
			"status":         "consumed",
			"appointment_id": appt.ID,
			"consumed_at":    time.Now().UTC(),
		}).Error; err != nil {
			return err
		}

		status, collected, mode := slotBookingPayment(&quote, appt.ExamFee)
		notes := strings.TrimSpace(appt.Notes)
		if notes != "" {
			notes += "\n\n" + note
		} else {
			notes = note
		}
		if err := tx.Model(&Appointment{}).Where("id = ?", appt.ID).Updates(map[string]interface{}{
			"prepaid_slot_id":  slot.ID,
			"payment_status":   status,
			"collected_amount": collected,
			"payment_mode":     mode,
			"notes":            notes,
		}).Error; err != nil {
			return err
		}
		if len(sessionQuotes) > 0 {
			if err := tx.Where("appointment_id = ?", appt.ID).Delete(&Quotation{}).Error; err != nil {
				return err
			}
		}
		return tx.Preload("Client").Preload("Subject").First(&out, appt.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AdjustClientSlotCount is a count-only correction (no money changes):
// "mark_used" marks the client's oldest free slot as used without linking an exam;
// "restore" frees the most recent slot that was marked used without an exam.
func (s *Service) AdjustClientSlotCount(clientID string, action string) (*PrepaidSlotSummary, error) {
	if _, err := s.GetClientByID(clientID); err != nil {
		return nil, err
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		switch action {
		case "mark_used":
			var slot ExamPrepaidSlot
			if err := tx.Where("client_id = ? AND status = ?", clientID, "available").
				Order("created_at ASC, id ASC").First(&slot).Error; err != nil {
				return errors.New("no free slot to mark as used")
			}
			return tx.Model(&ExamPrepaidSlot{}).Where("id = ?", slot.ID).Updates(map[string]interface{}{
				"status":      "consumed",
				"consumed_at": time.Now().UTC(),
			}).Error
		case "restore":
			// Only slots not tied to a real exam: marked used by hand, or held by a
			// cancelled exam (cancellations count as used unless staff waive it here).
			var slot ExamPrepaidSlot
			if err := tx.Where("client_id = ? AND status = ?", clientID, "consumed").
				Where("appointment_id IS NULL OR appointment_id IN (?)",
					tx.Model(&Appointment{}).Select("id").Where("status IN ?", []string{"cancelled", "canceled"})).
				Order("consumed_at DESC, id DESC").First(&slot).Error; err != nil {
				return errors.New("no slot to restore: every used slot belongs to an exam that is not cancelled")
			}
			if slot.AppointmentID != nil {
				return releaseSlotForCancelledAppointment(tx, *slot.AppointmentID)
			}
			return tx.Model(&ExamPrepaidSlot{}).Where("id = ?", slot.ID).Updates(map[string]interface{}{
				"status":      "available",
				"consumed_at": nil,
			}).Error
		default:
			return errors.New("action must be mark_used or restore")
		}
	})
	if err != nil {
		return nil, err
	}
	return s.GetClientPrepaidSlotSummary(clientID)
}

// slotBackedAppointmentCodes maps each appointment that used a prepaid slot to the code
// of the bulk invoice covering it.
func (s *Service) slotBackedAppointmentCodes(appointmentIDs []uint) map[uint]string {
	out := map[uint]string{}
	if len(appointmentIDs) == 0 {
		return out
	}
	type row struct {
		AppointmentID uint
		Code          string
		QuoteID       uint
	}
	var rows []row
	_ = s.db.Table("appointments").
		Select("appointments.id AS appointment_id, quotations.code AS code, quotations.id AS quote_id").
		Joins("JOIN exam_prepaid_slots ON exam_prepaid_slots.id = appointments.prepaid_slot_id").
		Joins("JOIN quotations ON quotations.id = exam_prepaid_slots.source_quotation_id").
		Where("appointments.id IN ?", appointmentIDs).
		Scan(&rows).Error
	for _, r := range rows {
		code := strings.TrimSpace(r.Code)
		if code == "" {
			code = fmt.Sprintf("%d", r.QuoteID)
		}
		out[r.AppointmentID] = code
	}
	return out
}

// appointmentUsesPrepaidSlot reports whether a booking is covered by a bulk invoice.
func appointmentUsesPrepaidSlot(db *gorm.DB, appointmentID uint) bool {
	var n int64
	_ = db.Model(&Appointment{}).Where("id = ? AND prepaid_slot_id IS NOT NULL", appointmentID).Count(&n).Error
	return n > 0
}
