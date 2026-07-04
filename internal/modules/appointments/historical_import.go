package appointments

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"my-app/internal/modules/subjects"

	"gorm.io/gorm"
)

type HistoricalImportRow struct {
	FirstName        string   `json:"first_name"`
	LastName         string   `json:"last_name"`
	Phone            string   `json:"phone"`
	EmployeeRef      string   `json:"employee_ref"`
	ScheduledAt      string   `json:"scheduled_at"` // RFC3339 formatted
	Status           string   `json:"status"`
	Verdict          string   `json:"verdict"` // legacy spreadsheet results (reference only)
	ExamTypeID       *uint    `json:"exam_type_id"`
	Price            *float64 `json:"price"`
	Gender           string   `json:"gender"`
	SpokenLanguage   string   `json:"spoken_language"`
	Experience       string   `json:"experience"`
	Email            string   `json:"email"`
	CaseLabel        string   `json:"case_label"`
	LegacyResults    string   `json:"legacy_results"`
	LegacyMailStatus string   `json:"legacy_mail_status"`
	SerialNo         string   `json:"serial_no"`
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

func (s *Service) BulkImportHistorical(
	clientID *uint,
	importMode string,
	examinerID uint,
	examFee float64,
	rows []HistoricalImportRow,
) (int, error) {
	importMode = strings.ToLower(strings.TrimSpace(importMode))
	if importMode == "" {
		importMode = "corporate"
	}
	if importMode != "individual" {
		if clientID == nil || *clientID == 0 {
			return 0, errors.New("client_id is required for corporate import")
		}
		if _, err := s.GetClientByID(strconv.FormatUint(uint64(*clientID), 10)); err != nil {
			return 0, err
		}
	}
	if len(rows) == 0 {
		return 0, errors.New("no rows to import")
	}

	importedCount := 0

	err := s.db.Transaction(func(tx *gorm.DB) error {
		txSvc := &Service{db: tx, storage: s.storage}

		for i, row := range rows {
			first := truncate(row.FirstName, 100)
			last := truncate(row.LastName, 100)
			if first == "" {
				first = "Examinee"
			}
			if last == "" {
				last = "Record"
			}

			rowClientID := uint(0)
			if importMode == "individual" {
				id, err := txSvc.findOrCreateIndividualClient(tx, first, last, row.Phone, row.Email, row.Gender)
				if err != nil {
					return fmt.Errorf("row %d: failed to create individual client: %w", i+1, err)
				}
				rowClientID = id
			} else {
				rowClientID = *clientID
			}

			// Create or find subject under the client account.
			var subj subjects.Subject
			var existingSubj subjects.Subject
			err := tx.Where("client_id = ? AND first_name = ? AND last_name = ?", rowClientID, first, last).First(&existingSubj).Error
			if err == nil {
				subj = existingSubj
				updates := map[string]interface{}{}
				if subj.Phone == "" && row.Phone != "" {
					updates["phone"] = truncate(row.Phone, 50)
				}
				if subj.EmployeeRef == "" && row.EmployeeRef != "" {
					updates["employee_ref"] = truncate(row.EmployeeRef, 100)
				}
				if subj.Gender == "" && row.Gender != "" {
					updates["gender"] = truncate(row.Gender, 20)
				}
				if subj.SpokenLanguage == "" && row.SpokenLanguage != "" {
					updates["spoken_language"] = truncate(row.SpokenLanguage, 100)
				}
				if subj.Email == "" && row.Email != "" {
					updates["email"] = truncate(row.Email, 255)
				}
				if len(updates) > 0 {
					tx.Model(&subj).Updates(updates)
				}
			} else {
				subj = subjects.Subject{
					ClientID:       &rowClientID,
					FirstName:      first,
					LastName:       last,
					Phone:          truncate(row.Phone, 50),
					EmployeeRef:    truncate(row.EmployeeRef, 100),
					Gender:         truncate(row.Gender, 20),
					SpokenLanguage: truncate(row.SpokenLanguage, 100),
					Email:          truncate(row.Email, 255),
				}
				if err := tx.Create(&subj).Error; err != nil {
					return fmt.Errorf("row %d: failed to create subject: %w", i+1, err)
				}
			}

			scheduledTime, err := time.Parse(time.RFC3339, row.ScheduledAt)
			if err != nil {
				scheduledTime = time.Now()
			}

			rowPrice := examFee
			if row.Price != nil {
				rowPrice = *row.Price
			}

			apptStatus, paymentStatus, _, _ := resolveHistoricalAppointmentStatus(row.Status)
			collectedAmount := rowPrice
			if apptStatus == "cancelled" || apptStatus == "pending" {
				collectedAmount = 0
				if apptStatus == "pending" {
					paymentStatus = "Unpaid"
				}
			}

			examTypeName := "Forensic Polygraph Screening"
			if row.ExamTypeID != nil && *row.ExamTypeID > 0 {
				var et struct {
					Name string `gorm:"column:name"`
				}
				if err := tx.Table("exam_types").Select("name").Where("id = ?", *row.ExamTypeID).First(&et).Error; err == nil && et.Name != "" {
					examTypeName = et.Name
				}
			}

			ref := strings.TrimSpace(row.EmployeeRef)
			if ref == "" {
				ref = strings.TrimSpace(row.SerialNo)
			}

			noteParts := []string{
				fmt.Sprintf("Imported historical record (%s)", examTypeName),
			}
			if ref != "" {
				noteParts = append(noteParts, fmt.Sprintf("Ref: %s", ref))
			}
			if row.CaseLabel != "" {
				noteParts = append(noteParts, fmt.Sprintf("Case: %s", row.CaseLabel))
			}
			if row.Status != "" {
				noteParts = append(noteParts, fmt.Sprintf("Legacy status: %s", row.Status))
			}
			legacyResults := strings.TrimSpace(row.LegacyResults)
			if legacyResults == "" {
				legacyResults = strings.TrimSpace(row.Verdict)
			}
			if legacyResults != "" {
				noteParts = append(noteParts, fmt.Sprintf("Legacy results: %s", legacyResults))
			}
			if row.LegacyMailStatus != "" {
				noteParts = append(noteParts, fmt.Sprintf("Legacy mail: %s", row.LegacyMailStatus))
			}
			if row.Experience != "" {
				noteParts = append(noteParts, fmt.Sprintf("Experience: %s", row.Experience))
			}
			if row.SpokenLanguage != "" {
				noteParts = append(noteParts, fmt.Sprintf("Language: %s", row.SpokenLanguage))
			}
			if row.Gender != "" {
				noteParts = append(noteParts, fmt.Sprintf("Gender: %s", row.Gender))
			}
			noteParts = append(noteParts, "Formal report pending in Polygraph — examiner must write, finalize, and send from Reports.")
			apptNotes := strings.Join(noteParts, " | ")

			appt := Appointment{
				ClientID:        rowClientID,
				SubjectID:       subj.ID,
				ExaminerID:      examinerID,
				ScheduledAt:     scheduledTime,
				Duration:        150,
				ExamFee:         rowPrice,
				PaymentMode:     "Bank Transfer",
				PaymentStatus:   paymentStatus,
				CollectedAmount: collectedAmount,
				Status:          apptStatus,
				Notes:           apptNotes,
			}
			txSvc.NormalizeNewAppointmentMoney(&appt)
			if err := tx.Create(&appt).Error; err != nil {
				return fmt.Errorf("row %d: failed to create appointment: %w", i+1, err)
			}

			if apptStatus != "pending" {
				if err := txSvc.ensureInvoiceForAppointment(&appt); err != nil {
					return fmt.Errorf("row %d: failed to create invoice: %w", i+1, err)
				}
			}

			// Create an exam shell for completed sessions so the examiner can write the
			// formal report in Report Builder. Do NOT pre-fill or lock a report.
			if apptStatus == "completed" {
				type ExamTemp struct {
					gorm.Model
					AppointmentID uint   `gorm:"uniqueIndex"`
					Status        string `gorm:"size:50"`
					ExamTypeID    *uint
					ClientID      uint
					SubjectID     uint
					ExaminerID    uint
					Date          time.Time
					Type          string `gorm:"size:150"`
					Notes         string `gorm:"type:text"`
				}
				examRec := ExamTemp{
					AppointmentID: appt.ID,
					Status:        "completed",
					ExamTypeID:    row.ExamTypeID,
					ClientID:      rowClientID,
					SubjectID:     subj.ID,
					ExaminerID:    examinerID,
					Date:          scheduledTime,
					Type:          examTypeName,
					Notes:         apptNotes,
				}
				if err := tx.Table("exams").Create(&examRec).Error; err != nil {
					return fmt.Errorf("row %d: failed to create exam: %w", i+1, err)
				}
				tx.Model(&appt).Update("exam_id", examRec.ID)
			}

			importedCount++
		}
		return nil
	})

	return importedCount, err
}
