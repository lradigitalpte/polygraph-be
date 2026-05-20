package forms

import (
	"time"

	"gorm.io/gorm"
)

// FormTemplate is a reusable clinical/legal form definition.
type FormTemplate struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Slug        string         `gorm:"size:80;uniqueIndex;not null" json:"slug"`
	Name        string         `gorm:"size:255;not null" json:"name"`
	Category    string         `gorm:"size:50;not null" json:"category"` // consent, privacy, legal, intake
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Audience    string         `gorm:"size:50;default:'all'" json:"audience"` // all, individual, corporate, examinee
	SchemaJSON  string         `gorm:"type:text;not null" json:"schema_json"`
	Version     int            `gorm:"default:1" json:"version"`
	Active      bool           `gorm:"default:true" json:"active"`
}

// FormRequest is one outbound form link sent to a client or examinee.
type FormRequest struct {
	ID               uint           `gorm:"primarykey" json:"id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	Token            string         `gorm:"size:64;uniqueIndex;not null" json:"token"`
	TemplateID       uint           `gorm:"index;not null" json:"template_id"`
	Template         FormTemplate   `gorm:"foreignKey:TemplateID" json:"template,omitempty"`
	ClientID         uint           `gorm:"index;not null" json:"client_id"`
	SubjectID        *uint          `gorm:"index" json:"subject_id,omitempty"`
	RecipientEmail   string         `gorm:"size:255;not null" json:"recipient_email"`
	RecipientName    string         `gorm:"size:255" json:"recipient_name,omitempty"`
	Status           string         `gorm:"size:30;default:'sent'" json:"status"` // sent, opened, completed, expired, cancelled
	SentAt           time.Time      `json:"sent_at"`
	OpenedAt         *time.Time     `json:"opened_at,omitempty"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	ExpiresAt        time.Time      `gorm:"index" json:"expires_at"`
	SubmittedData    string         `gorm:"type:text" json:"submitted_data,omitempty"`
	ClientDocumentID *uint          `json:"client_document_id,omitempty"`
	SubjectDocumentID *uint         `json:"subject_document_id,omitempty"`
	SentByEmail      string         `gorm:"size:255" json:"sent_by_email,omitempty"`
}
