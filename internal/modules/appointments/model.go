package appointments

import (
	"time"

	"gorm.io/gorm"
)

// Client is the organization or person requesting the test (Lawyer, Company, etc.)
type Client struct {
	ID                     uint           `gorm:"primarykey" json:"id"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	DeletedAt              gorm.DeletedAt `gorm:"index" json:"-"`
	Name                   string         `gorm:"size:255;not null" json:"name"`                   // Full Name or Company Name
	ClientType             string         `gorm:"size:50;default:'Individual'" json:"client_type"` // Individual, Corporate, Law Firm
	Gender                 string         `gorm:"size:20" json:"gender,omitempty"`                 // For individuals
	ContactPerson          string         `gorm:"size:255" json:"contact_person,omitempty"`        // For companies
	Phone                  string         `gorm:"size:50" json:"phone"`
	Email                  string         `gorm:"size:255;uniqueIndex" json:"email"`
	Address                string         `gorm:"type:text" json:"address"`
	TaxID                  string         `gorm:"size:100" json:"tax_id,omitempty"` // VAT or Company Reg Number
	PreferredPaymentMethod string         `gorm:"size:50;default:'Bank Transfer'" json:"preferred_payment_method"`
	Notes                  string         `gorm:"type:text" json:"notes"`
}

// Appointment represents a scheduled polygraph session
type Appointment struct {
	ID                uint           `gorm:"primarykey" json:"id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
	ClientID          uint           `json:"client_id"`
	Client            Client         `gorm:"foreignKey:ClientID" json:"client,omitempty"`
	SubjectID         uint           `json:"subject_id"`
	ExaminerID        uint           `json:"examiner_id"`
	ScheduledAt       time.Time      `json:"scheduled_at"`
	Duration          int            `json:"duration"` // In minutes
	ExamFee           float64        `gorm:"type:numeric(10,2);default:0" json:"exam_fee"`
	CollectedAmount   float64        `gorm:"type:numeric(10,2);default:0" json:"collected_amount"`
	Status            string         `gorm:"size:50;default:'pending'" json:"status"`        // pending, confirmed, cancelled, completed
	PaymentStatus     string         `gorm:"size:50;default:'Unpaid'" json:"payment_status"` // Paid, Partial, Unpaid
	PaymentMode       string         `gorm:"size:50" json:"payment_mode"`                    // Card, Bank Transfer, Cash
	QuestionsPrepared bool           `gorm:"default:false" json:"questions_prepared"`
	Notes             string         `gorm:"type:text" json:"notes"`
	ExamID            *uint          `json:"exam_id,omitempty"`
}
