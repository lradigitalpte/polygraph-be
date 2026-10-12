package bankaccounts

import (
	"time"

	"gorm.io/gorm"
)

// BankAccount is one of the lab's bank accounts that clients can pay into by transfer.
// Quotations copy the client-facing Details at the time they are created, so editing or
// archiving an account never changes an invoice that was already issued.
type BankAccount struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	// Label is internal only (e.g. "Mashreq – USD"); it is never printed for clients.
	Label         string `gorm:"size:100;not null" json:"label"`
	AccountName   string `gorm:"size:255;not null" json:"account_name"`
	BankName      string `gorm:"size:255;not null" json:"bank_name"`
	Currency      string `gorm:"size:10;not null;index" json:"currency"`
	AccountNumber string `gorm:"size:100" json:"account_number"`
	IBAN          string `gorm:"column:iban;size:64" json:"iban"`
	SwiftCode     string `gorm:"size:20" json:"swift_code"`
	// IsDefault marks the account pre-selected for new quotations in its currency
	// (at most one default per currency).
	IsDefault bool `gorm:"default:false" json:"is_default"`
	// Archived accounts stay on old invoices but can no longer be picked.
	Archived bool `gorm:"default:false" json:"archived"`
}

// Details is the client-facing copy of a bank account printed on invoices and emails.
type Details struct {
	AccountName   string `json:"account_name"`
	BankName      string `json:"bank_name"`
	Currency      string `json:"currency"`
	AccountNumber string `json:"account_number,omitempty"`
	IBAN          string `json:"iban,omitempty"`
	SwiftCode     string `json:"swift_code,omitempty"`
}

func (b BankAccount) ToDetails() Details {
	return Details{
		AccountName:   b.AccountName,
		BankName:      b.BankName,
		Currency:      b.Currency,
		AccountNumber: b.AccountNumber,
		IBAN:          b.IBAN,
		SwiftCode:     b.SwiftCode,
	}
}
