package accounting

import (
	"time"

	"gorm.io/gorm"
)

// ExpensePaymentMethod is a saved card or bank account used to pay expenses.
type ExpensePaymentMethod struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Label     string         `gorm:"size:120;not null" json:"label"`
	Type      string         `gorm:"size:32;not null" json:"type"` // credit_card, bank, cash, other
	LastFour  string         `gorm:"size:4" json:"last_four,omitempty"`
	BankName  string         `gorm:"size:120" json:"bank_name,omitempty"`
	IsDefault bool           `gorm:"default:false" json:"is_default"`
}

// ExpensePurchaseItem is a catalog line (e.g. office supplies) for quick expense entry.
type ExpensePurchaseItem struct {
	ID                 uint           `gorm:"primarykey" json:"id"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
	Name               string         `gorm:"size:255;not null" json:"name"`
	Description        string         `gorm:"type:text" json:"description"`
	Category           string         `gorm:"size:100" json:"category"`
	DefaultAmountExVat float64        `gorm:"type:numeric(10,2);default:0" json:"default_amount_ex_vat"`
	VatMode            string         `gorm:"size:16;default:'rate'" json:"vat_mode"` // rate, fixed, none
	VatRate            float64        `gorm:"type:numeric(6,3);default:0" json:"vat_rate"`
	VatAmountFixed     float64        `gorm:"type:numeric(10,2);default:0" json:"vat_amount_fixed"`
	Active             bool           `gorm:"default:true" json:"active"`
}
