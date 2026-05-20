package appointments

import "time"

// ClientDocument stores uploads and completed online forms for a client record.
type ClientDocument struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ClientID  uint      `gorm:"index;not null" json:"client_id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	Type      string    `gorm:"size:50;not null" json:"type"`   // upload, consent_form, intake_form, report, other
	Source    string    `gorm:"size:50;not null" json:"source"` // upload, online_form
	URL       string    `gorm:"size:500" json:"url,omitempty"`
	Hash      string    `gorm:"size:255" json:"hash,omitempty"`
	FormData  string    `gorm:"type:text" json:"form_data,omitempty"` // JSON payload for online forms
}
