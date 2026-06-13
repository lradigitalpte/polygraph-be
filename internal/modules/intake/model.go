package intake

import (
	"time"

	"gorm.io/gorm"
)

// IntakeRequest is a tokenised link sent to an organisation so they can
// submit their list of examinees without logging in.
type IntakeRequest struct {
	ID             uint           `gorm:"primarykey"                       json:"id"`
	CreatedAt      time.Time      `                                        json:"created_at"`
	UpdatedAt      time.Time      `                                        json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index"                            json:"-"`
	Token          string         `gorm:"size:64;uniqueIndex;not null"     json:"token"`
	ClientID       uint           `gorm:"index;not null"                   json:"client_id"`
	ClientName     string         `gorm:"size:255;not null"                json:"client_name"`
	RecipientEmail string         `gorm:"size:255;not null"                json:"recipient_email"`
	RecipientName  string         `gorm:"size:255"                         json:"recipient_name"`
	Message        string         `gorm:"type:text"                        json:"message"`
	ExpiresAt      time.Time      `gorm:"index"                            json:"expires_at"`
	Status         string         `gorm:"size:20;default:'pending'"        json:"status"` // pending, submitted, expired
	SubmittedAt    *time.Time     `                                        json:"submitted_at,omitempty"`
	// AgreedAt records when the submitter accepted the accuracy + data-use declaration.
	AgreedAt       *time.Time     `                                        json:"agreed_at,omitempty"`
	SentByEmail    string         `gorm:"size:255"                         json:"sent_by_email"`
	// JSON-encoded []subjects.Subject created on submission
	CreatedSubjects string `gorm:"type:text" json:"-"`
}
