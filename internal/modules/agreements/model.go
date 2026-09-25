package agreements

import (
	"time"

	"gorm.io/gorm"
)

// Agreement kinds. "custom" covers anything the lab adds beyond the three standard policies.
const (
	KindPayment      = "payment"
	KindReschedule   = "reschedule"
	KindCancellation = "cancellation"
	KindCustom       = "custom"
)

var validKinds = map[string]bool{
	KindPayment:      true,
	KindReschedule:   true,
	KindCancellation: true,
	KindCustom:       true,
}

// AgreementTemplate is an editable agreement managed in Settings. Editing the title or
// terms bumps Version; agreements already sent to clients keep their own snapshot, so
// a later edit never changes what a client has signed.
type AgreementTemplate struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Title     string         `gorm:"size:255;not null" json:"title"`
	Kind      string         `gorm:"size:30;not null;default:'custom'" json:"kind"`
	BodyHTML  string         `gorm:"type:text;not null" json:"body_html"`
	Version   int            `gorm:"not null;default:1" json:"version"`
	Active    bool           `gorm:"not null;default:true" json:"active"`
	SortOrder int            `gorm:"not null;default:0" json:"sort_order"`
	UpdatedBy string         `gorm:"size:255" json:"updated_by,omitempty"`
}

// Request statuses. "expired" is derived from ExpiresAt when a link is read, and stored
// the first time an overdue request is touched.
const (
	StatusSent     = "sent"
	StatusViewed   = "viewed"
	StatusSigned   = "signed"
	StatusDeclined = "declined"
	StatusExpired  = "expired"
	StatusVoided   = "voided"
)

// AgreementRequest is one signing link sent to a client. It bundles one or more
// agreements (Items), each a frozen copy of a template's terms at the time of sending.
type AgreementRequest struct {
	ID             uint           `gorm:"primarykey" json:"id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	Token          string         `gorm:"size:64;uniqueIndex;not null" json:"-"`
	ClientID       uint           `gorm:"index;not null" json:"client_id"`
	AppointmentID  *uint          `gorm:"index" json:"appointment_id,omitempty"`
	RecipientEmail string         `gorm:"size:255;not null" json:"recipient_email"`
	RecipientName  string         `gorm:"size:255" json:"recipient_name"`
	Status         string         `gorm:"size:20;not null;default:'sent';index" json:"status"`
	SentAt         time.Time      `json:"sent_at"`
	ViewedAt       *time.Time     `json:"viewed_at,omitempty"`
	SignedAt       *time.Time     `json:"signed_at,omitempty"`
	DeclinedAt     *time.Time     `json:"declined_at,omitempty"`
	DeclineReason  string         `gorm:"type:text" json:"decline_reason,omitempty"`
	VoidedAt       *time.Time     `json:"voided_at,omitempty"`
	VoidedBy       string         `gorm:"size:255" json:"voided_by,omitempty"`
	ExpiresAt      time.Time      `gorm:"index" json:"expires_at"`
	SentByEmail    string         `gorm:"size:255" json:"sent_by_email,omitempty"`
	// ContentHash is SHA-256 over the frozen terms, fixed at send time. SignatureHash binds
	// that content to the signer's name, signature image and signing time.
	ContentHash      string `gorm:"size:64;not null" json:"content_hash"`
	SignedName       string `gorm:"size:255" json:"signed_name,omitempty"`
	SignatureDataURL string `gorm:"type:text" json:"-"`
	SignatureHash    string `gorm:"size:64" json:"signature_hash,omitempty"`
	SignerIP         string `gorm:"size:64" json:"signer_ip,omitempty"`
	SignerUserAgent  string `gorm:"size:500" json:"signer_user_agent,omitempty"`
	ClientDocumentID *uint  `json:"client_document_id,omitempty"`
	// PDFData is the signed copy, generated once at signing and served for every download.
	PDFData   []byte                 `gorm:"column:pdf_data" json:"-"`
	PDFSHA256 string                 `gorm:"column:pdf_sha256;size:64" json:"pdf_sha256,omitempty"`
	Items     []AgreementRequestItem `gorm:"foreignKey:RequestID" json:"items,omitempty"`
}

// AgreementRequestItem is the frozen copy of one agreement inside a request.
type AgreementRequestItem struct {
	ID              uint       `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	RequestID       uint       `gorm:"index;not null" json:"request_id"`
	TemplateID      uint       `gorm:"index" json:"template_id"`
	TemplateVersion int        `json:"template_version"`
	Kind            string     `gorm:"size:30" json:"kind"`
	Title           string     `gorm:"size:255;not null" json:"title"`
	BodyHTML        string     `gorm:"type:text;not null" json:"body_html,omitempty"`
	SortOrder       int        `json:"sort_order"`
	AcceptedAt      *time.Time `json:"accepted_at,omitempty"`
}
