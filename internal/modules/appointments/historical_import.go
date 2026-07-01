package appointments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"my-app/internal/modules/subjects"
	"my-app/internal/utils"

	"gorm.io/gorm"
)

type HistoricalImportRow struct {
	FirstName      string   `json:"first_name"`
	LastName       string   `json:"last_name"`
	Phone          string   `json:"phone"`
	EmployeeRef    string   `json:"employee_ref"`
	ScheduledAt    string   `json:"scheduled_at"` // RFC3339 formatted
	Status         string   `json:"status"`       // "Completed" / "Failed" / "no show"
	Verdict        string   `json:"verdict"`      // "NDI" / "DI" / "Inconclusive"
	ExamTypeID     *uint    `json:"exam_type_id"`
	Price          *float64 `json:"price"`
	Gender         string   `json:"gender"`
	SpokenLanguage string   `json:"spoken_language"`
	Experience     string   `json:"experience"`
	Email          string   `json:"email"`
}

func (s *Service) BulkImportHistorical(
	clientID uint,
	examinerID uint,
	examFee float64,
	rows []HistoricalImportRow,
) (int, error) {
	if _, err := s.GetClientByID(strconv.FormatUint(uint64(clientID), 10)); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, errors.New("no rows to import")
	}

	importedCount := 0

	err := s.db.Transaction(func(tx *gorm.DB) error {
		txSvc := &Service{db: tx, storage: s.storage}

		for i, row := range rows {
			// 1. Create or Find Subject
			var subj subjects.Subject
			first := strings.TrimSpace(row.FirstName)
			last := strings.TrimSpace(row.LastName)
			if first == "" {
				first = "Examinee"
			}
			if last == "" {
				last = "Record"
			}

			// Check if subject already exists for this client with same name
			var existingSubj subjects.Subject
			err := tx.Where("client_id = ? AND first_name = ? AND last_name = ?", clientID, first, last).First(&existingSubj).Error
			if err == nil {
				subj = existingSubj
				// Update blank details if any
				updates := map[string]interface{}{}
				if subj.Phone == "" && row.Phone != "" {
					updates["phone"] = strings.TrimSpace(row.Phone)
				}
				if subj.EmployeeRef == "" && row.EmployeeRef != "" {
					updates["employee_ref"] = strings.TrimSpace(row.EmployeeRef)
				}
				if subj.Gender == "" && row.Gender != "" {
					updates["gender"] = strings.TrimSpace(row.Gender)
				}
				if subj.SpokenLanguage == "" && row.SpokenLanguage != "" {
					updates["spoken_language"] = strings.TrimSpace(row.SpokenLanguage)
				}
				if subj.Email == "" && row.Email != "" {
					updates["email"] = strings.TrimSpace(row.Email)
				}
				if len(updates) > 0 {
					tx.Model(&subj).Updates(updates)
				}
			} else {
				subj = subjects.Subject{
					ClientID:       &clientID,
					FirstName:      first,
					LastName:       last,
					Phone:          strings.TrimSpace(row.Phone),
					EmployeeRef:    strings.TrimSpace(row.EmployeeRef),
					Gender:         strings.TrimSpace(row.Gender),
					SpokenLanguage: strings.TrimSpace(row.SpokenLanguage),
					Email:          strings.TrimSpace(row.Email),
				}
				if err := tx.Create(&subj).Error; err != nil {
					return fmt.Errorf("row %d: failed to create subject: %w", i+1, err)
				}
			}

			// 2. Parse ScheduledAt
			scheduledTime, err := time.Parse(time.RFC3339, row.ScheduledAt)
			if err != nil {
				scheduledTime = time.Now()
			}

			// Determine status, payment status and collected amount
			apptStatus := "completed"
			paymentStatus := "Paid"
			
			rowPrice := examFee
			if row.Price != nil {
				rowPrice = *row.Price
			}
			
			collectedAmount := rowPrice
			if strings.ToLower(row.Status) == "no show" || strings.ToLower(row.Status) == "cancelled" {
				apptStatus = "cancelled"
				paymentStatus = "Unpaid"
				collectedAmount = 0
			}

			// Load the exam type name if ExamTypeID is supplied
			examTypeName := "Forensic Polygraph Screening"
			if row.ExamTypeID != nil && *row.ExamTypeID > 0 {
				var et struct {
					Name string `gorm:"column:name"`
				}
				if err := tx.Table("exam_types").Select("name").Where("id = ?", *row.ExamTypeID).First(&et).Error; err == nil && et.Name != "" {
					examTypeName = et.Name
				}
			}

			// Format notes dynamically to preserve gender, language, and experience
			noteParts := []string{
				fmt.Sprintf("Imported historical test record %s (%s)", row.EmployeeRef, examTypeName),
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
			apptNotes := strings.Join(noteParts, " | ")

			// 3. Create Appointment
			appt := Appointment{
				ClientID:        clientID,
				SubjectID:       subj.ID,
				ExaminerID:      examinerID,
				ScheduledAt:     scheduledTime,
				Duration:        150, // default exam duration
				ExamFee:         rowPrice,
				PaymentMode:     "Bank Transfer",
				PaymentStatus:   paymentStatus,
				CollectedAmount: collectedAmount,
				Status:          apptStatus,
				Notes:           apptNotes,
			}
			if err := tx.Create(&appt).Error; err != nil {
				return fmt.Errorf("row %d: failed to create appointment: %w", i+1, err)
			}

			// 4. Create Invoice/Quotation
			err = txSvc.ensureInvoiceForAppointment(&appt)
			if err != nil {
				return fmt.Errorf("row %d: failed to create invoice: %w", i+1, err)
			}

			// 5. If completed and we have a verdict, create the Exam and ExamReport
			if apptStatus == "completed" && row.Verdict != "" {
				type ExamTemp struct {
					gorm.Model
					AppointmentID uint   `gorm:"uniqueIndex"`
					Status        string `gorm:"size:50"`
					ExamTypeID    *uint
					ClientID      uint
					SubjectID     uint
					ExaminerID    uint
					Date          time.Time
				}
				examRec := ExamTemp{
					AppointmentID: appt.ID,
					Status:        "completed",
					ExamTypeID:    row.ExamTypeID,
					ClientID:      clientID,
					SubjectID:     subj.ID,
					ExaminerID:    examinerID,
					Date:          scheduledTime,
				}
				if err := tx.Table("exams").Create(&examRec).Error; err != nil {
					return fmt.Errorf("row %d: failed to create exam: %w", i+1, err)
				}

				// Update appointment to link it back to the exam
				tx.Model(&appt).Update("exam_id", examRec.ID)

				// Generate encrypted content for standard pre-employment screening questions
				type QAItem struct {
					Text       string `json:"text" xml:"text"`
					Answer     string `json:"answer" xml:"answer"`
					Evaluation string `json:"evaluation" xml:"evaluation"`
				}
				qaList := []QAItem{
					{Text: "Have you ever shared confidential company information with an unauthorized person?", Answer: "No", Evaluation: "No Reaction"},
					{Text: "Have you stolen money, leads or any property from a company you worked for?", Answer: "No", Evaluation: "No Reaction"},
					{Text: "Have you ever used company resources like leads or tools for your own personal gain or for someone else?", Answer: "No", Evaluation: "No Reaction"},
					{Text: "Is your purpose in applying for this position to intentionally damage or undermine the company?", Answer: "No", Evaluation: "No Reaction"},
				}
				if row.Verdict == "DI" {
					qaList[1].Answer = "No"
					qaList[1].Evaluation = "Reaction / Deceptive"
				}

				type StructuredReport struct {
					Purpose          string   `json:"purpose"`
					Instrument       string   `json:"instrument"`
					PreTestNotes     string   `json:"pre_test_notes"`
					Questions        []QAItem `json:"questions"`
					PostTestNotes    string   `json:"post_test_notes"`
					Conclusion       string   `json:"conclusion"`
					ReferenceNo      string   `json:"reference_no"`
					ExamDate         string   `json:"exam_date"`
					Section4FollowUp string   `json:"section_4_follow_up"`
				}

				conclusionText := "Analysis of the physiological records indicates no significant emotional or autonomic nervous system reactions to target items."
				if row.Verdict == "DI" {
					conclusionText = "Analysis of the physiological records indicates significant and consistent emotional or autonomic nervous system reactions to target items."
				} else if row.Verdict == "Inconclusive" {
					conclusionText = "Analysis of the physiological records is insufficient to formulate a definitive opinion."
				}

				reportContent := StructuredReport{
					Purpose:          fmt.Sprintf("Forensic Polygraph Assessment (%s)", examTypeName),
					Instrument:       "Lafayette LX6000",
					PreTestNotes:     "Examinee physical and mental health assessed as fit for testing. Legal rights and examination consent form explained and signed.",
					Questions:        qaList,
					PostTestNotes:    "Examinee cooperated and the test administration was as per procedure.",
					Conclusion:       conclusionText,
					ReferenceNo:      row.EmployeeRef,
					ExamDate:         scheduledTime.Format("02nd May 2006"),
					Section4FollowUp: "Nil",
				}

				contentJSON, _ := json.Marshal(reportContent)
				encrypted, err := utils.Encrypt(string(contentJSON))
				if err != nil {
					return fmt.Errorf("row %d: failed to encrypt report: %w", i+1, err)
				}

				hashSum := sha256.Sum256([]byte(encrypted))
				hashStr := hex.EncodeToString(hashSum[:])

				// Explicit struct mapping without standard gorm.Model to prevent SQLSTATE 42703 (updated_at does not exist)
				type ExamReportTemp struct {
					ID              uint `gorm:"primarykey"`
					CreatedAt       time.Time
					ExamID          uint `gorm:"uniqueIndex"`
					Verdict         string
					EncryptedReport string
					Hash            string
					IsLocked        bool
				}
				reportRec := ExamReportTemp{
					CreatedAt:       time.Now(),
					ExamID:          examRec.ID,
					Verdict:         row.Verdict,
					EncryptedReport: encrypted,
					Hash:            hashStr,
					IsLocked:        true,
				}
				if err := tx.Table("exam_reports").Create(&reportRec).Error; err != nil {
					return fmt.Errorf("row %d: failed to create exam report: %w", i+1, err)
				}
			}

			importedCount++
		}
		return nil
	})

	return importedCount, err
}
