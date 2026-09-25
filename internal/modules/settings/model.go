package settings

import "time"

// OrganizationSettings is a singleton row (id = 1) for lab branding and contact info.
type OrganizationSettings struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Name         string    `gorm:"size:255;not null" json:"name"`
	SupportEmail string    `gorm:"size:255" json:"support_email"`
	Phone        string    `gorm:"size:50" json:"phone"`
	Address      string    `gorm:"size:500" json:"address"`
	Website      string    `gorm:"size:255" json:"website"`
	// LogoDataURL is a small PNG/JPEG stored inline as a data: URL so public pages and
	// generated PDFs can use it without presigning a storage object.
	LogoDataURL           string  `gorm:"type:text" json:"logo_data_url"`
	Currency              string  `gorm:"size:10;default:'AED'" json:"currency"`
	UsdAedRate            float64 `gorm:"default:3.6725" json:"usd_aed_rate"`
	UsdGbpRate            float64 `gorm:"default:0.7850" json:"usd_gbp_rate"`
	UsdEurRate            float64 `gorm:"default:0.9250" json:"usd_eur_rate"`
	SundayBookingsEnabled bool    `gorm:"default:false" json:"sunday_bookings_enabled"`
}
