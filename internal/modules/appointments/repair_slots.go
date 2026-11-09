package appointments

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// RepairOrphanedPrepaidSlots fixes data left behind by the old DeleteAppointment, which
// soft-deleted bookings without returning their prepaid slot or voiding the session
// invoice. It is idempotent: once repaired, later runs match nothing. Safe to call on boot.
func RepairOrphanedPrepaidSlots(db *gorm.DB) (slotsReleased, invoicesVoided int64, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		// Voiding invoices must run first: it keys off appointments that are deleted
		// and were paid from a slot, independent of the slot's current state.
		res := tx.Model(&Quotation{}).
			Where("appointment_id IN (?)",
				tx.Unscoped().Model(&Appointment{}).Select("id").
					Where("deleted_at IS NOT NULL AND prepaid_slot_id IS NOT NULL")).
			Update("deleted_at", time.Now().UTC())
		if res.Error != nil {
			return res.Error
		}
		invoicesVoided = res.RowsAffected

		res = tx.Model(&ExamPrepaidSlot{}).
			Where("status = ? AND appointment_id IS NOT NULL", "consumed").
			Where("NOT EXISTS (SELECT 1 FROM appointments a WHERE a.id = exam_prepaid_slots.appointment_id AND a.deleted_at IS NULL)").
			Updates(map[string]interface{}{
				"status":         "available",
				"appointment_id": nil,
				"consumed_at":    nil,
			})
		if res.Error != nil {
			return res.Error
		}
		slotsReleased = res.RowsAffected
		return nil
	})
	return
}

// pioneervisionFix links one double-billed exam to the bulk-invoice slot that paid for it.
type pioneervisionFix struct {
	AppointmentID uint
	SlotID        uint
	SessionQuote  uint
	BulkCode      string
	SlotNo        int
	SlotTotal     int
}

// RepairPioneervisionDoubleBilling is a one-off data fix (2026-11-09) for client 38.
// Bulk quotes 009319 (5 exams) and 009332 (1 exam) were paid, but the 6 exams had also
// been booked with their own "paid" invoices, so the client was billed twice and 6 unused
// slots appeared. The owner confirmed the quotes are what was paid. This moves exams
// 299-304 onto slots 33-38 and removes session invoices 009326-009330 and 009334.
//
// It runs only if every row is still exactly as audited, so it is a no-op after the
// first successful run (or if staff already fixed it by hand). Returns true if applied.
func RepairPioneervisionDoubleBilling(db *gorm.DB) (bool, error) {
	const clientID = 38
	fixes := []pioneervisionFix{
		{299, 33, 326, "009319", 1, 5},
		{300, 34, 327, "009319", 2, 5},
		{301, 35, 328, "009319", 3, 5},
		{302, 36, 329, "009319", 4, 5},
		{303, 37, 330, "009319", 5, 5},
		{304, 38, 334, "009332", 1, 1},
	}
	applied := false
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, f := range fixes {
			var slot ExamPrepaidSlot
			if err := tx.Where("id = ? AND client_id = ? AND status = ? AND appointment_id IS NULL",
				f.SlotID, clientID, "available").First(&slot).Error; err != nil {
				return nil // already repaired or changed by hand: do nothing
			}
			var appt Appointment
			if err := tx.Where("id = ? AND client_id = ? AND prepaid_slot_id IS NULL", f.AppointmentID, clientID).
				First(&appt).Error; err != nil {
				return nil
			}
			var q Quotation
			if err := tx.Where("id = ? AND client_id = ? AND appointment_id = ?", f.SessionQuote, clientID, f.AppointmentID).
				First(&q).Error; err != nil {
				return nil
			}
			if quotationHasRecordedPayment(&q) {
				return nil // real money on the session invoice: needs a human
			}
		}

		for _, f := range fixes {
			var appt Appointment
			if err := tx.First(&appt, f.AppointmentID).Error; err != nil {
				return err
			}
			consumedAt := appt.ScheduledAt
			if err := tx.Model(&ExamPrepaidSlot{}).Where("id = ?", f.SlotID).Updates(map[string]interface{}{
				"status":         "consumed",
				"appointment_id": f.AppointmentID,
				"consumed_at":    consumedAt,
			}).Error; err != nil {
				return err
			}
			note := fmt.Sprintf("Paid from bulk invoice %s (slot %d of %d)", f.BulkCode, f.SlotNo, f.SlotTotal)
			notes := strings.TrimSpace(appt.Notes)
			if notes != "" {
				notes += "\n\n" + note
			} else {
				notes = note
			}
			if err := tx.Model(&Appointment{}).Where("id = ?", f.AppointmentID).Updates(map[string]interface{}{
				"prepaid_slot_id":  f.SlotID,
				"payment_status":   "Paid",
				"collected_amount": appt.ExamFee,
				"payment_mode":     "Bulk invoice " + f.BulkCode,
				"notes":            notes,
			}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&Quotation{}, f.SessionQuote).Error; err != nil {
				return err
			}
		}
		applied = true
		return nil
	})
	return applied, err
}
