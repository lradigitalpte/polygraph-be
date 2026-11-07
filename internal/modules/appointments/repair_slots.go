package appointments

import (
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
