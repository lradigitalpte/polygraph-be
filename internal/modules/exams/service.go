package exams

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"my-app/internal/email"
	"my-app/internal/models"
	"my-app/internal/storage"
	"my-app/internal/utils"
	"time"

	"github.com/jung-kurt/gofpdf"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	storage storage.Storage
}

func NewService(db *gorm.DB, storage storage.Storage) *Service {
	return &Service{db: db, storage: storage}
}

func reportAssetPath(names ...string) string {
	baseDirs := []string{
		"assets",
		filepath.Join("my-app", "assets"),
		filepath.Join("..", "my-app", "assets"),
		filepath.Join("frontend", "apps", "web", "public"),
		filepath.Join("..", "frontend", "apps", "web", "public"),
	}

	for _, dir := range baseDirs {
		for _, name := range names {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	return ""
}

func imageTypeFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "PNG"
	case ".jpg", ".jpeg", ".jfif":
		return "JPEG"
	default:
		return ""
	}
}

func (s *Service) CreateExam(exam *Exam) error {
	return s.db.Create(exam).Error
}

func (s *Service) GetAllExams() ([]Exam, error) {
	var exams []Exam
	err := s.db.Preload("Subject").Preload("ExamType").Find(&exams).Error
	return exams, err
}

func (s *Service) GetAllExamTypes() ([]ExamType, error) {
	var examTypes []ExamType
	err := s.db.Order("name ASC").Find(&examTypes).Error
	return examTypes, err
}

func (s *Service) CreateExamType(examType *ExamType) error {
	if examType.Name == "" {
		return errors.New("name is required")
	}
	if examType.Duration <= 0 {
		examType.Duration = 150
	}
	if examType.Price < 0 {
		return errors.New("price cannot be negative")
	}
	return s.db.Create(examType).Error
}

func (s *Service) UpdateExamType(id uint, updates map[string]interface{}) (*ExamType, error) {
	if duration, ok := updates["duration"].(float64); ok && duration <= 0 {
		return nil, errors.New("duration must be greater than 0")
	}
	if name, ok := updates["name"].(string); ok && name == "" {
		return nil, errors.New("name is required")
	}
	if price, ok := updates["price"].(float64); ok && price < 0 {
		return nil, errors.New("price cannot be negative")
	}

	var examType ExamType
	if err := s.db.First(&examType, id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&examType).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.First(&examType, id).Error; err != nil {
		return nil, err
	}
	return &examType, nil
}

func (s *Service) DeleteExamType(id uint) error {
	var count int64
	if err := s.db.Model(&Exam{}).Where("exam_type_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("cannot delete exam type that is already linked to exams")
	}
	return s.db.Delete(&ExamType{}, id).Error
}

func (s *Service) CreateReport(examID uint, verdict, content string) (*ExamReport, error) {
	encrypted, err := utils.Encrypt(content)
	if err != nil {
		return nil, err
	}

	hashSum := sha256.Sum256([]byte(encrypted))
	hashStr := hex.EncodeToString(hashSum[:])

	var report ExamReport
	err = s.db.Where("exam_id = ?", examID).First(&report).Error
	if err == nil {
		if report.IsLocked {
			return nil, fmt.Errorf("cannot modify a locked forensic report")
		}
		report.Verdict = verdict
		report.EncryptedReport = encrypted
		report.Hash = hashStr
		if err := s.db.Save(&report).Error; err != nil {
			return nil, err
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		report = ExamReport{
			ExamID:          examID,
			Verdict:         verdict,
			EncryptedReport: encrypted,
			Hash:            hashStr,
		}
		if err := s.db.Create(&report).Error; err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	s.db.Model(&Exam{}).Where("id = ?", examID).Update("status", "completed")
	return &report, nil
}

func (s *Service) GetReport(examID uint) (*ExamReport, string, error) {
	var report ExamReport
	if err := s.db.Where("exam_id = ?", examID).First(&report).Error; err != nil {
		return nil, "", err
	}

	decrypted, err := utils.Decrypt(report.EncryptedReport)
	return &report, decrypted, err
}

func (s *Service) UnlockReportForRevision(examID uint, actorID uint, reason string) (*ExamReport, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("override reason is required")
	}

	var report ExamReport
	if err := s.db.Where("exam_id = ?", examID).First(&report).Error; err != nil {
		return nil, err
	}
	if !report.IsLocked {
		return &report, nil
	}

	now := time.Now()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ExamReport{}).
			Where("id = ?", report.ID).
			Updates(map[string]any{"is_locked": false, "locked_at": nil, "signature_examiner": "", "signature_client": ""}).Error; err != nil {
			return err
		}

		if err := tx.Model(&SecureReportShare{}).
			Where("exam_report_id = ? AND expires_at > ?", report.ID, now).
			Update("expires_at", now).Error; err != nil {
			return err
		}

		payload, _ := json.Marshal(map[string]any{
			"exam_id": examID,
			"exam_report_id": report.ID,
			"reason": reason,
			"shares_expired_at": now.UTC().Format(time.RFC3339),
		})

		log := models.AuditLog{
			UserID: &actorID,
			Action: "REPORT_OVERRIDE_UNLOCK",
			Method: "POST",
			Path: fmt.Sprintf("/api/reports/%d/override-unlock", examID),
			Status: 200,
			Payload: string(payload),
		}
		return tx.Create(&log).Error
	}); err != nil {
		return nil, err
	}

	var unlocked ExamReport
	if err := s.db.Where("exam_id = ?", examID).First(&unlocked).Error; err != nil {
		return nil, err
	}
	return &unlocked, nil
}

// SignAndLockReport adds a signature and locks the report to make it immutable
func (s *Service) SignAndLockReport(examID uint, signature string, signatoryRole string) error {
	var report ExamReport
	if err := s.db.Where("exam_id = ?", examID).First(&report).Error; err != nil {
		return err
	}

	if report.IsLocked {
		return fmt.Errorf("report is already locked and cannot be signed again")
	}

	updates := map[string]interface{}{}
	if signatoryRole == "examiner" {
		updates["signature_examiner"] = signature
	} else if signatoryRole == "client" {
		updates["signature_client"] = signature
	} else {
		return fmt.Errorf("invalid signatory role")
	}

	// We can check if both have signed, or if examiner signing locks it.
	// For forensic integrity, if the examiner signs off on the final verdict, we lock it.
	if signatoryRole == "examiner" {
		updates["is_locked"] = true
		now := time.Now()
		updates["locked_at"] = &now
	}

	return s.db.Model(&report).Updates(updates).Error
}

func (s *Service) UploadDocument(ctx context.Context, examID uint, fileName string, fileType string, body io.Reader) (*Document, error) {
	// Create a hash object
	hasher := sha256.New()

	// We need to read the body twice: once for hashing and once for uploading.
	// For production, we'd use a temporary file or a pipe, but for now we'll buffer it.
	// Note: Large files should be handled via io.TeeReader to a temp file.
	fileBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file for hashing: %v", err)
	}

	// Compute Hash
	hasher.Write(fileBytes)
	hashSum := hex.EncodeToString(hasher.Sum(nil))

	// Upload to S3
	key := fmt.Sprintf("exams/%d/%s", examID, fileName)
	url, err := s.storage.UploadFile(ctx, key, bytes.NewReader(fileBytes), "application/octet-stream")
	if err != nil {
		return nil, err
	}

	doc := Document{
		ExamID: examID,
		Name:   fileName,
		Type:   fileType,
		URL:    url,
		Hash:   hashSum,
	}

	if err := s.db.Create(&doc).Error; err != nil {
		return nil, err
	}

	return &doc, nil
}

func (s *Service) GetDocuments(examID string) ([]Document, error) {
	var docs []Document
	err := s.db.Where("exam_id = ?", examID).Find(&docs).Error
	if err == nil {
		ctx := context.Background()
		for i := range docs {
			docs[i].URL = storage.SignedURLForStored(ctx, s.storage, docs[i].URL)
		}
	}
	return docs, err
}

func (s *Service) CreateReferral(ref *CaseReferral) error {
	return s.db.Create(ref).Error
}

func (s *Service) CreateAssessment(ass *ClinicalAssessment) error {
	return s.db.Create(ass).Error
}

func (s *Service) AddPhase(phase *ExamPhase) error {
	return s.db.Create(phase).Error
}

func (s *Service) GetIntelligence(examID string) (*Exam, error) {
	var exam Exam
	err := s.db.Preload("Subject").Preload("ExamType").Preload("Referral").Preload("Assessment").Preload("Phases").First(&exam, examID).Error
	return &exam, err
}

type appointmentLink struct {
	ID          uint `gorm:"primarykey"`
	ClientID    uint
	SubjectID   uint
	ExaminerID  uint
	ScheduledAt time.Time
	ExamID      *uint
	Notes       string `gorm:"type:text"`
}

func (appointmentLink) TableName() string { return "appointments" }

// signExamDocuments swaps the stored (private) document URLs for short-lived
// presigned URLs so the browser can actually open them from a private bucket.
func (s *Service) signExamDocuments(exam *Exam) {
	if exam == nil {
		return
	}
	ctx := context.Background()
	for i := range exam.Documents {
		exam.Documents[i].URL = storage.SignedURLForStored(ctx, s.storage, exam.Documents[i].URL)
	}
}

func (s *Service) GetExamByID(id string) (*Exam, error) {
	var exam Exam
	err := s.db.
		Preload("Subject").
		Preload("ExamType").
		Preload("Documents").
		Preload("Phases").
		First(&exam, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("exam not found")
		}
		return nil, err
	}
	s.signExamDocuments(&exam)
	return &exam, nil
}

func (s *Service) GetExamByAppointmentID(appointmentID string) (*Exam, error) {
	var exam Exam
	err := s.db.
		Where("appointment_id = ?", appointmentID).
		Preload("Subject").
		Preload("ExamType").
		Preload("Documents").
		Preload("Phases").
		First(&exam).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	s.signExamDocuments(&exam)
	return &exam, nil
}

func (s *Service) UpdateExam(id string, updates map[string]interface{}) (*Exam, error) {
	var exam Exam
	if err := s.db.First(&exam, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("exam not found")
		}
		return nil, err
	}

	allowed := map[string]bool{"notes": true, "status": true, "type": true}
	safe := make(map[string]interface{})
	for key, value := range updates {
		if allowed[key] {
			safe[key] = value
		}
	}
	if len(safe) > 0 {
		if err := s.db.Model(&exam).Updates(safe).Error; err != nil {
			return nil, err
		}
	}

	// Keep the linked appointment's status in sync so admin/ops views and the
	// examiner's documentation always agree on where the session stands.
	if newStatus, ok := safe["status"].(string); ok && exam.AppointmentID != nil {
		if apptStatus := examToAppointmentStatus(newStatus); apptStatus != "" {
			s.db.Model(&appointmentLink{}).Where("id = ?", *exam.AppointmentID).Update("status", apptStatus)
		}
	}

	return s.GetExamByID(id)
}

// examToAppointmentStatus maps an exam workflow status to the appointment status the
// rest of the app (dashboard, exam history, billing ledger) reads.
func examToAppointmentStatus(examStatus string) string {
	switch examStatus {
	case "scheduled":
		return "pending"
	case "in_progress":
		return "confirmed"
	case "completed":
		return "completed"
	case "cancelled":
		return "cancelled"
	default:
		return ""
	}
}

func (s *Service) StartDocumentationForAppointment(appointmentID string) (*Exam, error) {
	var appt appointmentLink
	if err := s.db.First(&appt, appointmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("appointment not found")
		}
		return nil, err
	}

	if appt.ExamID != nil && *appt.ExamID > 0 {
		return s.GetExamByID(strconv.FormatUint(uint64(*appt.ExamID), 10))
	}

	apptID := appt.ID
	exam := Exam{
		ClientID:      appt.ClientID,
		SubjectID:     appt.SubjectID,
		ExaminerID:    appt.ExaminerID,
		AppointmentID: &apptID,
		Date:          appt.ScheduledAt,
		Status:        "in_progress",
		Type:          "Polygraph examination",
		Notes:         appt.Notes,
	}
	if err := s.db.Create(&exam).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&appointmentLink{}).Where("id = ?", appt.ID).Update("exam_id", exam.ID).Error; err != nil {
		return nil, err
	}

	return s.GetExamByID(strconv.FormatUint(uint64(exam.ID), 10))
}

type StructuredReport struct {
	Purpose           string `json:"purpose"`
	Instrument        string `json:"instrument"`
	PreTestNotes      string `json:"pre_test_notes"`
	Questions         []struct {
		Text       string `json:"text"`
		Answer     string `json:"answer"`
		Evaluation string `json:"evaluation"`
	} `json:"questions"`
	PostTestNotes     string `json:"post_test_notes"`
	Conclusion        string `json:"conclusion"`
	ReferenceNo       string `json:"reference_no"`
	ExamDate          string `json:"exam_date"`
	Section4FollowUp  string `json:"section_4_follow_up"`
	LimeToneNotes     string `json:"limestone_notes"`
	PreTestPhaseText  string `json:"pre_test_phase_text"`
	ExamPhaseText     string `json:"exam_phase_text"`
	OpinionPhaseText  string `json:"opinion_phase_text"`
}

func GenerateEncryptedPDF(verdict string, content string, subjectName string, examType string, clientName string, password string) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetProtection(gofpdf.CnProtectPrint, password, password)
	
	// Parse report content
	var reportData StructuredReport
	isStructured := json.Unmarshal([]byte(content), &reportData) == nil

	// Set header func
	pdf.SetHeaderFunc(func() {
		// Draw logo
		if logoPath := reportAssetPath("logo.png"); logoPath != "" {
			pdf.Image(logoPath, 15, 12, 45, 0, false, imageTypeFromPath(logoPath), 0, "")
		} else {
			// Fallback text if image not found
			pdf.SetFont("Helvetica", "B", 12)
			pdf.SetTextColor(220, 38, 38)
			pdf.Text(15, 18, "POLYGRAPH UAE")
		}
		
		pdf.SetTextColor(120, 120, 120)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.Text(155, 18, "STAFF IN CONFIDENCE")
		
		pdf.SetDrawColor(200, 200, 200)
		pdf.SetLineWidth(0.3)
		pdf.Line(15, 23, 195, 23)
	})

	// Set footer func
	pdf.SetFooterFunc(func() {
		// Draw logos
		yPos := float64(262)
		if apaPath := reportAssetPath("americanpolygraphassociation.png"); apaPath != "" {
			pdf.Image(apaPath, 82, yPos, 14, 14, false, imageTypeFromPath(apaPath), 0, "")
		}
		if sapPath := reportAssetPath("singaporeassociationofpolygraph.jpg", "singaporeassociationofpolygraph.jfif"); sapPath != "" {
			pdf.Image(sapPath, 102, yPos+1.5, 23, 11, false, imageTypeFromPath(sapPath), 0, "")
		}
		
		pdf.SetTextColor(100, 100, 100)
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.SetXY(15, yPos + 17)
		pdf.CellFormat(180, 4, "Polygraph International HR Consultancy LLC | Office 401-41, Deyaar building, Al Barsha 1, Dubai, United Arab Emirates", "", 0, "C", false, 0, "")
		pdf.Ln(4)
		pdf.CellFormat(180, 4, "Website: www.polygraph.ae | Email: info@polygraph.ae", "", 0, "C", false, 0, "")
		
		pdf.SetTextColor(120, 120, 120)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.SetXY(15, yPos + 26)
		pdf.CellFormat(180, 4, "STAFF IN CONFIDENCE", "", 0, "C", false, 0, "")
	})

	pdf.SetMargins(15, 32, 15)
	pdf.SetAutoPageBreak(true, 42) // leaves space for footer
	pdf.AddPage()
	
	pdf.SetTextColor(0, 0, 0)
	
	// Page 1 Content
	// EXAMINEE INFORMATION
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Cell(0, 6, "EXAMINEE INFORMATION")
	pdf.Ln(7)
	
	// Detail fields table/list
	pdf.SetFont("Helvetica", "B", 9)
	pdf.Cell(30, 6, "OUR REF")
	pdf.SetFont("Helvetica", "", 9)
	refNo := "PIN/CONF/2026/001"
	if isStructured && reportData.ReferenceNo != "" {
		refNo = reportData.ReferenceNo
	}
	pdf.Cell(0, 6, ": "+refNo)
	pdf.Ln(6)
	
	pdf.SetFont("Helvetica", "B", 9)
	pdf.Cell(30, 6, "DATE")
	pdf.SetFont("Helvetica", "", 9)
	examDate := time.Now().Format("02nd May 2006")
	if isStructured && reportData.ExamDate != "" {
		examDate = reportData.ExamDate
	}
	pdf.Cell(0, 6, ": "+examDate)
	pdf.Ln(6)
	
	pdf.SetFont("Helvetica", "B", 9)
	pdf.Cell(30, 6, "EXAMINEE")
	pdf.SetFont("Helvetica", "", 9)
	pdf.Cell(0, 6, ": "+subjectName)
	pdf.Ln(10)
	
	if isStructured {
		// SECTION 1: PRE-EXAMINATION PHASE
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Cell(0, 6, "SECTION 1: PRE-EXAMINATION PHASE")
		pdf.Ln(7)
		
		pdf.SetFont("Helvetica", "", 9.5)
		preTestText := fmt.Sprintf("On %s at about 14:00 hrs (Dubai Time), I commenced to administer a polygraph examination to the above subject.\n\nA screening polygraph test was administered as part of a pre-employment test for %s.", examDate, clientName)
		if reportData.PreTestPhaseText != "" {
			preTestText = reportData.PreTestPhaseText
		}
		pdf.MultiCell(0, 5, preTestText, "", "L", false)
		pdf.Ln(5)
		
		preNotes := "Examinee physical and mental health assessed as fit for testing. Legal rights and examination consent form explained and signed."
		if reportData.PreTestNotes != "" {
			preNotes = reportData.PreTestNotes
		}
		pdf.MultiCell(0, 5, preNotes, "", "L", false)
		pdf.Ln(10)
		
		// SECTION 2: EXAMINATION PHASE
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Cell(0, 6, "SECTION 2: EXAMINATION PHASE")
		pdf.Ln(7)
		
		pdf.SetFont("Helvetica", "", 9.5)
		examPhaseText := "During the examination phase, the relevant and comparison questions were administered to subject with a set of relevant questions. His verbal responses to the relevant questions were as indicated:"
		if reportData.ExamPhaseText != "" {
			examPhaseText = reportData.ExamPhaseText
		}
		pdf.MultiCell(0, 5, examPhaseText, "", "L", false)
		pdf.Ln(6)
		
		// Q&A Table
		if len(reportData.Questions) > 0 {
			pdf.SetFont("Helvetica", "B", 8)
			pdf.SetFillColor(250, 250, 250)
			pdf.CellFormat(15, 7, "S/N", "1", 0, "C", true, 0, "")
			pdf.CellFormat(125, 7, "Questions", "1", 0, "L", true, 0, "")
			pdf.CellFormat(40, 7, "Examinee Response", "1", 1, "C", true, 0, "")
			
			pdf.SetFont("Helvetica", "", 9)
			for idx, q := range reportData.Questions {
				x, y := pdf.GetX(), pdf.GetY()

				questionText := strings.TrimSpace(q.Text)
				if questionText == "" {
					questionText = "-"
				}

				pdf.SetFont("Helvetica", "I", 9)
				lines := pdf.SplitLines([]byte(questionText), 119)
				h := float64(len(lines)) * 6
				if h < 8 {
					h = 8
				}

				pdf.SetXY(x, y)
				pdf.SetFont("Helvetica", "", 9)
				pdf.CellFormat(15, h, strconv.Itoa(idx+1), "1", 0, "C", false, 0, "")

				pdf.SetXY(x+15, y)
				pdf.SetFont("Helvetica", "I", 9)
				pdf.MultiCell(125, 6, questionText, "1", "L", false)

				pdf.SetXY(x+140, y)
				pdf.SetFont("Helvetica", "B", 9)
				pdf.CellFormat(40, h, q.Answer, "1", 1, "C", false, 0, "")
			}
			pdf.Ln(8)
		}
		
		// Limestone / test process text
		limestoneNotes := "The examination was conducted with a Limestone Technologies Computerised Polygraph, recording the blood pressure, pulse rate, galvanic skin response and breathing pattern of the subject.\n\nFour polygrams, including 1 acquaintance and 3 official tests were recorded, and the process ended at about 15:35 hrs (Dubai Time)."
		if reportData.LimeToneNotes != "" {
			limestoneNotes = reportData.LimeToneNotes
		}
		pdf.SetFont("Helvetica", "", 9.5)
		pdf.MultiCell(0, 5, limestoneNotes, "", "L", false)
		pdf.Ln(10)
		
		// SECTION 3: OPINION OF EXAMINER
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Cell(0, 6, "SECTION 3: OPINION OF EXAMINER")
		pdf.Ln(7)
		
		opinionText := fmt.Sprintf("Based on the diagnostic evaluations and analysis of the polygrams, I am in the opinion that the examination on %s as %s.", subjectName, verdict)
		if reportData.OpinionPhaseText != "" {
			opinionText = reportData.OpinionPhaseText
		}
		pdf.SetFont("Helvetica", "", 9.5)
		pdf.MultiCell(0, 5, opinionText, "", "L", false)
		pdf.Ln(5)
		
		postNotes := "Examinee cooperated and the test administration was as per procedure."
		if reportData.PostTestNotes != "" {
			postNotes = reportData.PostTestNotes
		}
		pdf.MultiCell(0, 5, postNotes, "", "L", false)
		pdf.Ln(6)
		
		// Result badge
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Cell(25, 6, "Result: ")
		
		pdf.SetFont("Helvetica", "B", 11)
		if verdict == "DI" {
			pdf.SetTextColor(200, 0, 0)
			pdf.Cell(0, 6, "NOT TRUTHFUL")
		} else if verdict == "NDI" {
			pdf.SetTextColor(0, 150, 0)
			pdf.Cell(0, 6, "TRUTHFUL / NO DECEPTION INDICATED")
		} else {
			pdf.SetTextColor(100, 100, 100)
			pdf.Cell(0, 6, "INCONCLUSIVE")
		}
		pdf.SetTextColor(0, 0, 0)
		pdf.Ln(12)
		
		// SECTION 4: FOLLOW-UP BY REQUESTING AGENCY
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Cell(0, 6, "SECTION 4: FOLLOW-UP BY REQUESTING AGENCY")
		pdf.Ln(7)
		
		followUp := "Nil"
		if reportData.Section4FollowUp != "" {
			followUp = reportData.Section4FollowUp
		}
		pdf.SetFont("Helvetica", "", 9.5)
		pdf.MultiCell(0, 5, followUp, "", "L", false)
		
	} else {
		// Fallback for unstructured text
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Cell(0, 6, "REPORT FINDINGS & CONCLUSION")
		pdf.Ln(7)
		pdf.SetFont("Helvetica", "", 9.5)
		pdf.MultiCell(0, 5.5, content, "", "L", false)
	}

	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func generatePasscode() string {
	const chars = "0123456789"
	result := make([]byte, 6)
	for i := 0; i < 6; i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		result[i] = chars[num.Int64()]
	}
	return string(result)
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) GetConsolidatedReportStats() (map[string]any, error) {
	var totalShares int64
	var totalNDI int64
	var totalDI int64
	var totalInconclusive int64

	// Count shares
	if err := s.db.Model(&SecureReportShare{}).Count(&totalShares).Error; err != nil {
		return nil, err
	}

	// Count report verdicts
	if err := s.db.Model(&ExamReport{}).Where("verdict = ?", "NDI").Count(&totalNDI).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&ExamReport{}).Where("verdict = ?", "DI").Count(&totalDI).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&ExamReport{}).Where("verdict = ? OR verdict = ?", "Inconclusive", "INCONCLUSIVE").Count(&totalInconclusive).Error; err != nil {
		return nil, err
	}

	return map[string]any{
		"total_reports":      totalShares,
		"ndi_count":          totalNDI,
		"di_count":           totalDI,
		"inconclusive_count": totalInconclusive,
	}, nil
}

func (s *Service) ListSecureShares(search string, clientID uint, subjectID uint) ([]SecureReportShare, error) {
	var shares []SecureReportShare
	query := s.db.Preload("Subject").Preload("ExamReport")

	if clientID > 0 {
		query = query.Where("client_id = ?", clientID)
	}

	if subjectID > 0 {
		query = query.Where("subject_id = ?", subjectID)
	}

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		query = query.Joins("JOIN subjects ON subjects.id = secure_report_shares.subject_id").
			Where("LOWER(subjects.first_name) LIKE ? OR LOWER(subjects.last_name) LIKE ? OR LOWER(secure_report_shares.recipient_email) LIKE ?", like, like, like)
	}

	err := query.Order("created_at DESC").Find(&shares).Error
	return shares, err
}

func (s *Service) CreateSecureShare(reportID uint, recipientEmail string) (*SecureReportShare, error) {
	// Fetch report
	var report ExamReport
	if err := s.db.First(&report, reportID).Error; err != nil {
		return nil, fmt.Errorf("report not found: %w", err)
	}

	// Fetch exam details to build name/metadata
	var exam Exam
	if err := s.db.Preload("Subject").Preload("ExamType").First(&exam, report.ExamID).Error; err != nil {
		return nil, fmt.Errorf("exam not found: %w", err)
	}

	// Decrypt report content
	decryptedContent, err := utils.Decrypt(report.EncryptedReport)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt report: %w", err)
	}

	// Fetch client name from database dynamically
	var clientName string = "Corporate Client"
	type clientTemp struct {
		Name string
	}
	var cTemp clientTemp
	if err := s.db.Table("clients").Select("name").Where("id = ?", exam.ClientID).Scan(&cTemp).Error; err == nil && cTemp.Name != "" {
		clientName = cTemp.Name
	}

	// Generate passcode & token
	passcode := generatePasscode()
	token := generateToken()

	// Generate PDF
	subjectName := fmt.Sprintf("%s %s", exam.Subject.FirstName, exam.Subject.LastName)
	examTypeName := exam.Type
	if examTypeName == "" && exam.ExamType != nil {
		examTypeName = exam.ExamType.Name
	}
	if examTypeName == "" {
		examTypeName = "Polygraph Forensic Exam"
	}

	pdfBytes, err := GenerateEncryptedPDF(report.Verdict, decryptedContent, subjectName, examTypeName, clientName, passcode)
	if err != nil {
		return nil, fmt.Errorf("failed to generate encrypted PDF: %w", err)
	}

	// Upload PDF to S3/Storage
	fileName := fmt.Sprintf("share-report-%d-%s.pdf", reportID, token[:8])
	key := fmt.Sprintf("exams/reports/%s", fileName)
	pdfURL, err := s.storage.UploadFile(context.Background(), key, bytes.NewReader(pdfBytes), "application/pdf")
	if err != nil {
		return nil, fmt.Errorf("failed to upload PDF to storage: %w", err)
	}

	// Save GORM record
	share := SecureReportShare{
		ExamReportID:   reportID,
		ClientID:       exam.ClientID,
		SubjectID:      exam.SubjectID,
		RecipientEmail: recipientEmail,
		Token:          token,
		Password:       passcode,
		PdfURL:         pdfURL,
		Status:         "sent",
		ExpiresAt:      time.Now().Add(7 * 24 * time.Hour), // 7 days expiration
	}

	if err := s.db.Create(&share).Error; err != nil {
		return nil, fmt.Errorf("failed to save share: %w", err)
	}

	// Email delivery
	frontendURL := strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}
	secureLink := fmt.Sprintf("%s/shared/report/%s", strings.TrimSuffix(frontendURL, "/"), token)

	subjectLine := fmt.Sprintf("CONFIDENTIAL: Polygraph Report for %s", subjectName)
	emailBody := fmt.Sprintf(
		"Hello,\n\nYour polygraph forensic report for %s is attached to this email as a password-protected PDF.\n\nTo view the password and securely unlock this document, please click the link below:\n%s\n\nFor security reasons, this link will expire in 7 days.\n\nBest regards,\nPolygraph Forensic System Team",
		subjectName,
		secureLink,
	)

	// Send it!
	_ = email.SendWithAttachment(recipientEmail, subjectLine, emailBody, fmt.Sprintf("Forensic_Report_%s.pdf", exam.Subject.LastName), pdfBytes)

	return &share, nil
}

func (s *Service) GetSecureReportShareByToken(token string) (*SecureReportShare, error) {
	var share SecureReportShare
	err := s.db.Preload("Subject").Preload("ExamReport").Where("token = ? AND expires_at > ?", token, time.Now()).First(&share).Error
	if err != nil {
		return nil, err
	}
	return &share, nil
}

func (s *Service) RegenerateSecureReportShare(id uint) (*SecureReportShare, error) {
	var share SecureReportShare
	if err := s.db.Preload("Subject").Preload("ExamReport").First(&share, id).Error; err != nil {
		return nil, err
	}

	// Fetch exam to get decryptions
	var exam Exam
	if err := s.db.Preload("Subject").Preload("ExamType").First(&exam, share.ExamReport.ExamID).Error; err != nil {
		return nil, err
	}

	decryptedContent, err := utils.Decrypt(share.ExamReport.EncryptedReport)
	if err != nil {
		return nil, err
	}

	var clientName string = "Corporate Client"
	type clientTemp struct {
		Name string
	}
	var cTemp clientTemp
	if err := s.db.Table("clients").Select("name").Where("id = ?", exam.ClientID).Scan(&cTemp).Error; err == nil && cTemp.Name != "" {
		clientName = cTemp.Name
	}

	// Rotate passcode & token
	passcode := generatePasscode()
	token := generateToken()

	// Regenerate PDF
	subjectName := fmt.Sprintf("%s %s", exam.Subject.FirstName, exam.Subject.LastName)
	examTypeName := exam.Type
	if examTypeName == "" && exam.ExamType != nil {
		examTypeName = exam.ExamType.Name
	}
	if examTypeName == "" {
		examTypeName = "Polygraph Forensic Exam"
	}

	pdfBytes, err := GenerateEncryptedPDF(share.ExamReport.Verdict, decryptedContent, subjectName, examTypeName, clientName, passcode)
	if err != nil {
		return nil, err
	}

	// Upload rotated PDF
	fileName := fmt.Sprintf("share-report-%d-%s.pdf", share.ExamReportID, token[:8])
	key := fmt.Sprintf("exams/reports/%s", fileName)
	pdfURL, err := s.storage.UploadFile(context.Background(), key, bytes.NewReader(pdfBytes), "application/pdf")
	if err != nil {
		return nil, err
	}

	// Update record
	updates := map[string]any{
		"token":      token,
		"password":   passcode,
		"pdf_url":    pdfURL,
		"status":     "sent",
		"expires_at": time.Now().Add(7 * 24 * time.Hour),
	}

	if err := s.db.Model(&share).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Fetch updated record
	if err := s.db.Preload("Subject").Preload("ExamReport").First(&share, id).Error; err != nil {
		return nil, err
	}

	// Send email again
	frontendURL := strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}
	secureLink := fmt.Sprintf("%s/shared/report/%s", strings.TrimSuffix(frontendURL, "/"), token)

	subjectLine := fmt.Sprintf("CONFIDENTIAL (UPDATED): Polygraph Report for %s", subjectName)
	emailBody := fmt.Sprintf(
		"Hello,\n\nYour polygraph forensic report link has been regenerated. The report is attached to this email as a password-protected PDF.\n\nTo view the new password and unlock the document, please click the secure link below:\n%s\n\nFor security reasons, this link will expire in 7 days.\n\nBest regards,\nPolygraph Forensic System Team",
		secureLink,
	)

	_ = email.SendWithAttachment(share.RecipientEmail, subjectLine, emailBody, fmt.Sprintf("Forensic_Report_%s.pdf", exam.Subject.LastName), pdfBytes)

	return &share, nil
}



