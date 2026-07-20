package exams

import (
	"errors"
	"my-app/internal/modules/subjects"
	"time"

	"gorm.io/gorm"
)

// ExamType defines a reusable exam protocol that can be managed from settings.
type ExamType struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Name        string         `gorm:"size:150;uniqueIndex;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Category    string         `gorm:"size:100" json:"category"`
	Duration    int            `gorm:"default:150" json:"duration"`
	Price       float64        `gorm:"type:numeric(10,2);default:0" json:"price"`
	Active      bool           `gorm:"default:true" json:"active"`
}

// Exam represents a polygraph session
type Exam struct {
	ID            uint                `gorm:"primarykey" json:"id"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	DeletedAt     gorm.DeletedAt      `gorm:"index" json:"-"`
	ClientID      uint                `json:"client_id"`
	SubjectID     uint                `json:"subject_id"`
	Subject       subjects.Subject    `gorm:"foreignKey:SubjectID" json:"subject,omitempty"`
	ExaminerID    uint                `json:"examiner_id"`
	AppointmentID *uint               `json:"appointment_id,omitempty"`
	Date          time.Time           `json:"date"`
	ExamTypeID    *uint               `json:"exam_type_id,omitempty"`
	ExamType      *ExamType           `gorm:"foreignKey:ExamTypeID" json:"exam_type,omitempty"`
	Type          string              `gorm:"size:100" json:"type"`
	Status        string              `gorm:"size:50;default:'scheduled'" json:"status"`
	Notes         string              `gorm:"type:text" json:"notes"`
	Questions     []ExamQuestion      `gorm:"foreignKey:ExamID" json:"questions,omitempty"`
	Report        *ExamReport         `gorm:"foreignKey:ExamID" json:"report,omitempty"`
	Documents     []Document          `gorm:"foreignKey:ExamID" json:"documents,omitempty"`
	Referral      *CaseReferral       `gorm:"foreignKey:ExamID" json:"referral,omitempty"`
	Assessment    *ClinicalAssessment `gorm:"foreignKey:ExamID" json:"assessment,omitempty"`
	Phases        []ExamPhase         `gorm:"foreignKey:ExamID" json:"phases,omitempty"`
}

// ExamQuestion tracks questions asked during a session
type ExamQuestion struct {
	ID       uint   `gorm:"primarykey" json:"id"`
	ExamID   uint   `json:"exam_id"`
	Text     string `gorm:"type:text;not null" json:"text"`
	Response string `gorm:"size:50" json:"response"` // Truthful, Deceptive, Inconclusive
}

// ExamReport is the final forensic verdict
type ExamReport struct {
	ID                 uint       `gorm:"primarykey" json:"id"`
	ExamID             uint       `gorm:"uniqueIndex" json:"exam_id"`
	Verdict            string     `gorm:"size:100" json:"verdict"` // DI, NDI, Inconclusive
	EncryptedReport    string     `gorm:"type:text" json:"-"`
	Hash               string     `gorm:"size:255" json:"hash"` // For integrity checking
	CreatedAt          time.Time  `json:"created_at"`
	SignatureExaminer  string     `gorm:"type:text" json:"signature_examiner,omitempty"` // Cryptographic digital signature (base64)
	SignatureImage     string     `gorm:"type:text" json:"-"`
	SignerExaminerID   uint       `json:"signer_examiner_id,omitempty"`
	SignerName         string     `gorm:"size:255" json:"signer_name,omitempty"`
	SignerTitle        string     `gorm:"size:255" json:"signer_title,omitempty"`
	SignerOrganization string     `gorm:"size:255" json:"signer_organization,omitempty"`
	SignedAt           *time.Time `json:"signed_at,omitempty"`
	SignatureClient    string     `gorm:"type:text" json:"signature_client,omitempty"` // Cryptographic digital signature (base64)
	IsLocked           bool       `gorm:"default:false" json:"is_locked"`
	LockedAt           *time.Time `json:"locked_at,omitempty"`
}

// BeforeUpdate prevents modifications if the report is locked
func (r *ExamReport) BeforeUpdate(tx *gorm.DB) (err error) {
	// Let's check if the existing record is locked
	var existing ExamReport

	// If r.ID is 0, it means it's a map update and we should get the ID from the statement condition
	var id interface{} = r.ID
	if r.ID == 0 {
		// Attempt to extract from DB context (advanced GORM) or just block it
		// For safety, let's just use the current model ID if possible
		if tx.Statement.Dest != nil {
			if model, ok := tx.Statement.Dest.(*ExamReport); ok && model.ID != 0 {
				id = model.ID
			}
		}
	}

	if id == uint(0) || id == interface{}(uint(0)) {
		// We still need to block if we can't verify locking status in a bulk update
		return errors.New("cannot bulk update forensic reports; must specify ID to verify lock status")
	}

	if err := tx.Model(&ExamReport{}).Where("id = ?", id).First(&existing).Error; err == nil {
		if existing.IsLocked {
			return errors.New("cannot modify a locked forensic report")
		}
	}
	return nil
}

// Document stores charts, consent forms, or report files
type Document struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExamID    uint      `json:"exam_id"`
	Name      string    `gorm:"size:255" json:"name"`
	Type      string    `gorm:"size:50" json:"type"`  // chart, consent, report, audio
	URL       string    `gorm:"size:500" json:"url"`  // Link to S3 or internal storage
	Hash      string    `gorm:"size:255" json:"hash"` // Integrity check
}

// CaseReferral is the intelligence context provided by the client
type CaseReferral struct {
	ID              uint   `gorm:"primarykey" json:"id"`
	ExamID          uint   `gorm:"uniqueIndex" json:"exam_id"`
	ReasonForExam   string `gorm:"type:text;not null" json:"reason_for_exam"`
	TargetedIssues  string `gorm:"type:text" json:"targeted_issues"` // Specific questions to investigate
	BackgroundIntel string `gorm:"type:text" json:"background_intel"`
}

// ClinicalAssessment tracks the subject's fitness for the exam
type ClinicalAssessment struct {
	ID             uint    `gorm:"primarykey" json:"id"`
	ExamID         uint    `gorm:"uniqueIndex" json:"exam_id"`
	SleepHours     float32 `json:"sleep_hours"`
	Medications    string  `gorm:"type:text" json:"medications"`
	MedicalHistory string  `gorm:"type:text" json:"medical_history"`
	BloodPressure  string  `gorm:"size:20" json:"blood_pressure"`
	PulseRate      int     `json:"pulse_rate"`
	EmotionalState string  `gorm:"size:100" json:"emotional_state"`
	FitnessVerdict string  `gorm:"size:50" json:"fitness_verdict"` // Fit, Unfit, Conditional
}

// ExamPhase tracks the professional timeline
type ExamPhase struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	ExamID    uint      `json:"exam_id"`
	Name      string    `gorm:"size:100" json:"name"` // Pre-Test, In-Test, Post-Test
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Notes     string    `gorm:"type:text" json:"notes"`
}

// SecureReportShare tracks secure external distribution links for final reports
type SecureReportShare struct {
	ID               uint             `gorm:"primarykey" json:"id"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	DeletedAt        gorm.DeletedAt   `gorm:"index" json:"-"`
	ExamReportID     uint             `json:"exam_report_id"`
	ExamReport       *ExamReport      `gorm:"foreignKey:ExamReportID" json:"exam_report,omitempty"`
	ClientID         uint             `json:"client_id"`
	SubjectID        uint             `json:"subject_id"`
	Subject          subjects.Subject `gorm:"foreignKey:SubjectID" json:"subject,omitempty"`
	RecipientEmail   string           `gorm:"size:255;not null" json:"recipient_email"`
	Token            string           `gorm:"size:255;uniqueIndex;not null" json:"token"`
	Password         string           `gorm:"size:100;not null" json:"password"`
	ProtectionMode   string           `gorm:"size:30;default:'password';not null" json:"protection_mode"` // password, secure_link
	PdfURL           string           `gorm:"size:500" json:"pdf_url"`
	VerificationCode string           `gorm:"size:64;uniqueIndex" json:"verification_code"`
	PDFHash          string           `gorm:"size:64" json:"-"`
	Status           string           `gorm:"size:50;default:'sent'" json:"status"` // sent, viewed
	ExpiresAt        time.Time        `json:"expires_at"`
	ArchivedAt       *time.Time       `gorm:"index" json:"archived_at,omitempty"`
}
