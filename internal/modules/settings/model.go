package settings

import "time"

// OrganizationSettings is a singleton row (id = 1) for lab branding and contact info.
type OrganizationSettings struct {
	ID                    uint      `gorm:"primarykey" json:"id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	Name                  string    `gorm:"size:255;not null" json:"name"`
	SupportEmail          string    `gorm:"size:255" json:"support_email"`
	Address               string    `gorm:"size:500" json:"address"`
	Currency              string    `gorm:"size:10;default:'AED'" json:"currency"`
	UsdAedRate            float64   `gorm:"default:3.6725" json:"usd_aed_rate"`
	UsdGbpRate            float64   `gorm:"default:0.7850" json:"usd_gbp_rate"`
	UsdEurRate            float64   `gorm:"default:0.9250" json:"usd_eur_rate"`
	SundayBookingsEnabled bool      `gorm:"default:false" json:"sunday_bookings_enabled"`
}
