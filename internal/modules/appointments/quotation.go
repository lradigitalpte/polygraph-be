package appointments

import (
	"time"

	"gorm.io/gorm"
)

// Quotation represents a billable quote issued to a client.
type Quotation struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	ClientID        uint           `json:"client_id"`
	Client          Client         `gorm:"foreignKey:ClientID" json:"client,omitempty"`
	AppointmentID   *uint          `json:"appointment_id,omitempty"`
	Code            string         `gorm:"size:50;uniqueIndex" json:"code"`
	Title           string         `gorm:"size:255;not null" json:"title"`
	Description     string         `gorm:"type:text" json:"description"`
	Amount          float64        `gorm:"type:numeric(10,2);not null" json:"amount"`
	CollectedAmount float64        `gorm:"type:numeric(10,2);default:0" json:"collected_amount"`
	Status          string         `gorm:"size:50;default:'Draft'" json:"status"`
	SentAt          *time.Time     `json:"sent_at,omitempty"`
	SentToEmail     string         `gorm:"size:255" json:"sent_to_email,omitempty"`
	EmailSubject    string         `gorm:"size:255" json:"email_subject,omitempty"`
	EmailBody       string         `gorm:"type:text" json:"email_body,omitempty"`
	Currency        string         `gorm:"size:10;default:'USD'" json:"currency"`

	// Stripe Checkout (last generated payment link + applied session tracking).
	StripeCheckoutSessionID  string `gorm:"size:255" json:"stripe_checkout_session_id,omitempty"`
	StripePaymentIntentID    string `gorm:"size:255" json:"stripe_payment_intent_id,omitempty"`
	StripePaymentLinkURL     string `gorm:"size:512" json:"stripe_payment_link_url,omitempty"`
	StripeAppliedSessionIDs  string `gorm:"type:text" json:"stripe_applied_session_ids,omitempty"` // JSON array of applied Checkout Session IDs
}
