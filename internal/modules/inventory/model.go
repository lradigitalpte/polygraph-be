package inventory

import "time"

type InventoryItem struct {
	ID             uint       `gorm:"primarykey" json:"id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Name           string     `gorm:"size:255;not null" json:"name"`
	SerialNumber   string     `gorm:"size:255" json:"serial_number"`
	Category       string     `gorm:"size:100;default:'Equipment'" json:"category"`
	Status         string     `gorm:"size:50;default:'Active'" json:"status"`
	Quantity       int        `gorm:"default:1" json:"quantity"`
	Location       string     `gorm:"size:255" json:"location"`
	PurchaseDate   *time.Time `json:"purchase_date"`
	WarrantyExpiry *time.Time `json:"warranty_expiry"`
	CalibrationDue *time.Time `json:"calibration_due"`
	ExpirationDate *time.Time `json:"expiration_date"`
	Notes          string     `gorm:"type:text" json:"notes"`
}
