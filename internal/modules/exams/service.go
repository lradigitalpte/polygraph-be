package exams

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"my-app/internal/email"
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

	report := ExamReport{
		ExamID:          examID,
		Verdict:         verdict,
		EncryptedReport: encrypted,
		Hash:            hashStr,
	}

	if err := s.db.Create(&report).Error; err != nil {
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

func GenerateEncryptedPDF(verdict string, content string, subjectName string, examType string, clientName string, password string) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetProtection(gofpdf.CnProtectPrint, password, password)
	pdf.AddPage()
	
	// Branded dark top bar simulation
	pdf.SetFillColor(0, 0, 0)
	pdf.Rect(0, 0, 210, 30, "F")
	
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Text(15, 20, "POLYGRAPH FORENSIC SYSTEM")
	
	// Report Details Section
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(25) // move below header
	
	pdf.SetFont("Helvetica", "B", 14)
	pdf.Cell(0, 10, "Confidential Forensic Examination Report")
	pdf.Ln(12)
	
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Cell(40, 7, "Examinee Name:")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 7, subjectName)
	pdf.Ln(7)
	
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Cell(40, 7, "Exam Type:")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 7, examType)
	pdf.Ln(7)
	
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Cell(40, 7, "Requesting Client:")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 7, clientName)
	pdf.Ln(7)
	
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Cell(40, 7, "Verdict:")
	pdf.SetFont("Helvetica", "B", 10)
	if verdict == "DI" {
		pdf.SetTextColor(200, 0, 0) // red
		pdf.Cell(0, 7, "Deception Indicated (DI)")
	} else if verdict == "NDI" {
		pdf.SetTextColor(0, 150, 0) // green
		pdf.Cell(0, 7, "No Deception Indicated (NDI)")
	} else {
		pdf.SetTextColor(100, 100, 100) // gray
		pdf.Cell(0, 7, "Inconclusive")
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(12)
	
	// Divider
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(8)
	
	// Report Content
	pdf.SetFont("Helvetica", "B", 11)
	pdf.Cell(0, 7, "Professional Findings & Conclusion:")
	pdf.Ln(8)
	
	pdf.SetFont("Helvetica", "", 10)
	pdf.MultiCell(0, 6, content, "", "L", false)
	
	pdf.Ln(15)
	pdf.SetFont("Helvetica", "I", 9)
	pdf.Cell(0, 5, "This document is encrypted for strict confidentiality and data protection.")
	pdf.Ln(5)
	pdf.Cell(0, 5, fmt.Sprintf("Report Generated on %s", time.Now().Format("Jan 02, 2006")))
	
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

func (s *Service) ListSecureShares(search string, clientID uint) ([]SecureReportShare, error) {
	var shares []SecureReportShare
	query := s.db.Preload("Subject").Preload("ExamReport")

	if clientID > 0 {
		query = query.Where("client_id = ?", clientID)
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
