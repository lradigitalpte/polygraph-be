package accounting

import (
	"time"

	"gorm.io/gorm"
)

// Expense is a purchase / operating cost for input-VAT tracking.
type Expense struct {
	ID              uint           `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	ExpenseDate     time.Time      `gorm:"not null;index" json:"expense_date"`
	Vendor          string         `gorm:"size:255" json:"vendor"`
	Description     string         `gorm:"type:text" json:"description"`
	Category        string         `gorm:"size:100" json:"category"`
	AmountExVat     float64        `gorm:"type:numeric(10,2);not null" json:"amount_ex_vat"`
	VatRate         float64        `gorm:"type:numeric(6,3);default:0" json:"vat_rate"`
	VatAmount       float64        `gorm:"type:numeric(10,2);default:0" json:"vat_amount"`
	AmountIncVat    float64        `gorm:"type:numeric(10,2);not null" json:"amount_inc_vat"`
	Currency        string         `gorm:"size:10;default:'AED'" json:"currency"`
	ReceiptRef      string         `gorm:"size:512" json:"receipt_ref,omitempty"`
	CreatedByUserID *uint          `json:"created_by_user_id,omitempty"`
}
