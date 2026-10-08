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
	ReceiptFileName string         `gorm:"size:255" json:"receipt_file_name,omitempty"`
	ReceiptURL      string         `gorm:"size:512" json:"receipt_url,omitempty"`
	ReceiptStorageKey string       `gorm:"size:512" json:"-"`

	PurchaseItemID   *uint                `json:"purchase_item_id,omitempty"`
	PurchaseItem     *ExpensePurchaseItem `gorm:"foreignKey:PurchaseItemID" json:"purchase_item,omitempty"`
	PaymentMethodID  *uint                `json:"payment_method_id,omitempty"`
	PaymentMethod    *ExpensePaymentMethod `gorm:"foreignKey:PaymentMethodID" json:"payment_method,omitempty"`
	PaymentType      string               `gorm:"size:32" json:"payment_type,omitempty"`
	PaymentLabel     string               `gorm:"size:120" json:"payment_label,omitempty"`
	PaymentLastFour  string               `gorm:"size:4" json:"payment_last_four,omitempty"`

	CreatedByUserID *uint `json:"created_by_user_id,omitempty"`
}
