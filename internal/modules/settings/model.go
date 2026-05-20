package settings

import "time"

// OrganizationSettings is a singleton row (id = 1) for lab branding and contact info.
type OrganizationSettings struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Name         string    `gorm:"size:255;not null" json:"name"`
	SupportEmail string    `gorm:"size:255" json:"support_email"`
	Address      string    `gorm:"size:500" json:"address"`
}
