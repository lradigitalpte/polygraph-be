package exams

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"my-app/internal/storage"
	"my-app/internal/utils"
	"time"

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
