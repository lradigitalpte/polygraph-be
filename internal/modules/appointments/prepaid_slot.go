package appointments

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ExamPrepaidSlot is one redeemable examination credit from a paid bulk invoice.
type ExamPrepaidSlot struct {
	ID                uint           `gorm:"primarykey" json:"id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
	ClientID          uint           `gorm:"index;not null" json:"client_id"`
	SourceQuotationID uint           `gorm:"index;not null" json:"source_quotation_id"`
	SourceQuotation   Quotation      `gorm:"foreignKey:SourceQuotationID" json:"source_quotation,omitempty"`
	Status            string         `gorm:"size:20;default:'available';index" json:"status"` // available, consumed, void
	AppointmentID     *uint          `gorm:"index" json:"appointment_id,omitempty"`
	ConsumedAt        *time.Time     `json:"consumed_at,omitempty"`
}

type PrepaidSlotSummary struct {
	Available int `json:"available"`
	Consumed  int `json:"consumed"`
	Total     int `json:"total"`
}

func isCorporateClientType(clientType string) bool {
	t := strings.ToLower(strings.TrimSpace(clientType))
	return t == "corporate" || t == "law firm"
}

func quotationFullyPaid(quote *Quotation) bool {
	if quote == nil || quote.Amount <= 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(quote.Status), "Completed") {
		return true
	}
	return quote.CollectedAmount >= quote.Amount
}

func slotQuantityForQuotation(quote *Quotation) int {
	if quote == nil {
		return 0
	}
	qty := quote.LineQuantity
	if qty <= 0 {
		qty = 1
	}
	return qty
}

// ensureSlotsForPaidQuotation mints prepaid slots when a standalone invoice is fully paid.
func (s *Service) ensureSlotsForPaidQuotation(tx *gorm.DB, quote *Quotation) error {
	if tx == nil {
		tx = s.db
	}
	if quote == nil || quote.ID == 0 {
		return nil
	}
	if quote.AppointmentID != nil {
		return nil
	}
	if !quotationFullyPaid(quote) {
		return nil
	}

	qty := slotQuantityForQuotation(quote)
	var existing int64
	if err := tx.Model(&ExamPrepaidSlot{}).Where("source_quotation_id = ?", quote.ID).Count(&existing).Error; err != nil {
		return err
	}
	if existing >= int64(qty) {
		return nil
	}
	toCreate := qty - int(existing)

	rows := make([]ExamPrepaidSlot, toCreate)
	for i := range rows {
		rows[i] = ExamPrepaidSlot{
			ClientID:          quote.ClientID,
			SourceQuotationID: quote.ID,
			Status:            "available",
		}
	}
	return tx.Create(&rows).Error
}

// MintPrepaidSlotsForQuotation creates missing slots for a fully paid standalone invoice
// (e.g. payment recorded before slots existed, or line_quantity was raised after payment).
// lineQuantity, when set, updates how many examinations this invoice covers before minting.
// alreadyConsumed marks exams completed before slots existed (FIFO consume after mint).
func (s *Service) MintPrepaidSlotsForQuotation(quotationID string, lineQuantity *int, alreadyConsumed int) (*PrepaidSlotSummary, error) {
	var quote Quotation
	if err := s.db.First(&quote, quotationID).Error; err != nil {
		return nil, errors.New("quotation not found")
	}
	if quote.AppointmentID != nil {
		return nil, errors.New("only standalone bulk invoices can mint examination slots")
	}
	if !quotationFullyPaid(&quote) {
		return nil, errors.New("invoice must be fully paid before creating examination slots")
	}

	var existingSlots int64
	if err := s.db.Model(&ExamPrepaidSlot{}).Where("source_quotation_id = ?", quote.ID).Count(&existingSlots).Error; err != nil {
		return nil, err
	}
	if existingSlots > 0 {
		if alreadyConsumed > 0 {
			if err := s.markPrepaidSlotsAlreadyConsumed(s.db, quote.ID, alreadyConsumed); err != nil {
				return nil, err
			}
			clientID := strconv.FormatUint(uint64(quote.ClientID), 10)
			return s.GetClientPrepaidSlotSummary(clientID)
		}
		return nil, errors.New("prepaid examination slots already exist for this invoice")
	}

	if lineQuantity != nil && *lineQuantity > 0 {
		if err := s.db.Model(&Quotation{}).Where("id = ?", quote.ID).Update("line_quantity", *lineQuantity).Error; err != nil {
			return nil, err
		}
		quote.LineQuantity = *lineQuantity
	}
	if alreadyConsumed < 0 {
		alreadyConsumed = 0
	}
	qty := slotQuantityForQuotation(&quote)
	if alreadyConsumed > qty {
		return nil, fmt.Errorf("already used (%d) cannot exceed examinations on invoice (%d)", alreadyConsumed, qty)
	}

	if err := s.ensureSlotsForPaidQuotation(s.db, &quote); err != nil {
		return nil, err
	}
	if alreadyConsumed > 0 {
		if err := s.markPrepaidSlotsAlreadyConsumed(s.db, quote.ID, alreadyConsumed); err != nil {
			return nil, err
		}
	}
	clientID := strconv.FormatUint(uint64(quote.ClientID), 10)
	return s.GetClientPrepaidSlotSummary(clientID)
}

func (s *Service) markPrepaidSlotsAlreadyConsumed(tx *gorm.DB, sourceQuotationID uint, targetConsumed int) error {
	if tx == nil {
		tx = s.db
	}
	if targetConsumed <= 0 {
		return nil
	}

	var consumed int64
	if err := tx.Model(&ExamPrepaidSlot{}).
		Where("source_quotation_id = ? AND status = ?", sourceQuotationID, "consumed").
		Count(&consumed).Error; err != nil {
		return err
	}
	need := targetConsumed - int(consumed)
	if need <= 0 {
		return nil
	}

	var available []ExamPrepaidSlot
	if err := tx.Where("source_quotation_id = ? AND status = ?", sourceQuotationID, "available").
		Order("created_at ASC, id ASC").
		Limit(need).
		Find(&available).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for i := range available {
		if err := tx.Model(&available[i]).Updates(map[string]interface{}{
			"status":      "consumed",
			"consumed_at": now,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) quotationHasPrepaidSlots(quotationID uint) (bool, error) {
	var count int64
	if err := s.db.Model(&ExamPrepaidSlot{}).Where("source_quotation_id = ?", quotationID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) populateQuotationPrepaidFlags(quote *Quotation) {
	if quote == nil {
		return
	}
	has, err := s.quotationHasPrepaidSlots(quote.ID)
	if err == nil {
		quote.PrepaidSlotsMinted = has
	}
}

func (s *Service) GetClientPrepaidSlots(clientID string) ([]ExamPrepaidSlot, error) {
	if _, err := s.GetClientByID(clientID); err != nil {
		return nil, err
	}
	var slots []ExamPrepaidSlot
	err := s.db.Preload("SourceQuotation").
		Where("client_id = ?", clientID).
		Order("created_at ASC, id ASC").
		Find(&slots).Error
	return slots, err
}

func (s *Service) GetClientPrepaidSlotSummary(clientID string) (*PrepaidSlotSummary, error) {
	if _, err := s.GetClientByID(clientID); err != nil {
		return nil, err
	}
	var summary PrepaidSlotSummary
	type row struct {
		Status string
		Count  int
	}
	var rows []row
	if err := s.db.Model(&ExamPrepaidSlot{}).
		Select("status, COUNT(*) as count").
		Where("client_id = ?", clientID).
		Group("status").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		switch strings.ToLower(strings.TrimSpace(r.Status)) {
		case "available":
			summary.Available = r.Count
		case "consumed":
			summary.Consumed = r.Count
		}
	}
	summary.Total = summary.Available + summary.Consumed
	return &summary, nil
}

func prepaidSlotNote(quote Quotation, slotIndex int) string {
	total := slotQuantityForQuotation(&quote)
	code := strings.TrimSpace(quote.Code)
	if code == "" {
		code = fmt.Sprintf("%d", quote.ID)
	}
	return fmt.Sprintf("Paid from bulk invoice %s (slot %d of %d)", code, slotIndex, total)
}

func (s *Service) redeemPrepaidSlotForBooking(tx *gorm.DB, clientID uint) (*ExamPrepaidSlot, string, error) {
	var slot ExamPrepaidSlot
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("SourceQuotation").
		Where("client_id = ? AND status = ?", clientID, "available").
		Order("created_at ASC, id ASC").
		First(&slot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", nil
		}
		return nil, "", err
	}

	quote := slot.SourceQuotation
	if quote.ID == 0 {
		if err := tx.First(&quote, slot.SourceQuotationID).Error; err != nil {
			return nil, "", err
		}
	}

	var consumedCount int64
	if err := tx.Model(&ExamPrepaidSlot{}).
		Where("source_quotation_id = ? AND status = ?", quote.ID, "consumed").
		Count(&consumedCount).Error; err != nil {
		return nil, "", err
	}
	note := prepaidSlotNote(quote, int(consumedCount)+1)
	return &slot, note, nil
}

func (s *Service) applyPrepaidSlotInvoiceNote(appt *Appointment) error {
	if appt == nil || appt.PrepaidSlotID == nil || appt.ID == 0 {
		return nil
	}
	var slot ExamPrepaidSlot
	if err := s.db.Preload("SourceQuotation").First(&slot, *appt.PrepaidSlotID).Error; err != nil {
		return nil
	}
	quote := slot.SourceQuotation
	if quote.ID == 0 {
		_ = s.db.First(&quote, slot.SourceQuotationID).Error
	}
	var consumedCount int64
	_ = s.db.Model(&ExamPrepaidSlot{}).
		Where("source_quotation_id = ? AND status = ?", quote.ID, "consumed").
		Count(&consumedCount).Error
	note := prepaidSlotNote(quote, int(consumedCount))
	var sessionQuote Quotation
	if err := s.db.Where("appointment_id = ?", appt.ID).First(&sessionQuote).Error; err != nil {
		return nil
	}
	return s.db.Model(&Quotation{}).Where("id = ?", sessionQuote.ID).Update("description", note).Error
}
