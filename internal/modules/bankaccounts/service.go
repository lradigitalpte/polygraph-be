package bankaccounts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"my-app/internal/database"
	"my-app/internal/models"
)

type Service struct {
	db *gorm.DB
}

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

type Input struct {
	Label         string `json:"label"`
	AccountName   string `json:"account_name"`
	BankName      string `json:"bank_name"`
	Currency      string `json:"currency"`
	AccountNumber string `json:"account_number"`
	IBAN          string `json:"iban"`
	SwiftCode     string `json:"swift_code"`
	IsDefault     bool   `json:"is_default"`
	Archived      bool   `json:"archived"`
}

func (s *Service) List() ([]BankAccount, error) {
	var rows []BankAccount
	err := s.db.Order("archived ASC, currency ASC, is_default DESC, label ASC").Find(&rows).Error
	return rows, err
}

func (s *Service) Create(input Input, actorID *uint) (*BankAccount, error) {
	row, err := normalize(input)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := clearOtherDefaults(tx, &row); err != nil {
			return err
		}
		return writeAudit(tx, actorID, "BANK_ACCOUNT_CREATED", "POST", "/api/settings/bank-accounts", nil, &row)
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) Update(id uint, input Input, actorID *uint) (*BankAccount, error) {
	var before BankAccount
	if err := s.db.First(&before, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("bank account not found")
		}
		return nil, err
	}
	next, err := normalize(input)
	if err != nil {
		return nil, err
	}
	next.ID = before.ID
	next.CreatedAt = before.CreatedAt
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&BankAccount{}).Where("id = ?", id).Updates(map[string]interface{}{
			"label":          next.Label,
			"account_name":   next.AccountName,
			"bank_name":      next.BankName,
			"currency":       next.Currency,
			"account_number": next.AccountNumber,
			"iban":           next.IBAN,
			"swift_code":     next.SwiftCode,
			"is_default":     next.IsDefault,
			"archived":       next.Archived,
		}).Error; err != nil {
			return err
		}
		if err := clearOtherDefaults(tx, &next); err != nil {
			return err
		}
		return writeAudit(tx, actorID, "BANK_ACCOUNT_UPDATED", "PATCH", fmt.Sprintf("/api/settings/bank-accounts/%d", id), &before, &next)
	})
	if err != nil {
		return nil, err
	}
	var saved BankAccount
	if err := s.db.First(&saved, id).Error; err != nil {
		return nil, err
	}
	return &saved, nil
}

func (s *Service) Delete(id uint, actorID *uint) error {
	var before BankAccount
	if err := s.db.First(&before, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("bank account not found")
		}
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&BankAccount{}, id).Error; err != nil {
			return err
		}
		return writeAudit(tx, actorID, "BANK_ACCOUNT_DELETED", "DELETE", fmt.Sprintf("/api/settings/bank-accounts/%d", id), &before, nil)
	})
}

// DefaultsFor returns the default account for a currency (if any) as invoice details.
func DefaultsFor(db *gorm.DB, currency string) []Details {
	var rows []BankAccount
	if err := db.Where("currency = ? AND is_default = ? AND archived = ?", strings.ToUpper(strings.TrimSpace(currency)), true, false).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]Details, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ToDetails())
	}
	return out
}

// DetailsByIDs resolves picked accounts in the order given; archived or missing ids are an error.
func DetailsByIDs(db *gorm.DB, ids []uint) ([]Details, error) {
	out := make([]Details, 0, len(ids))
	seen := map[uint]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		var row BankAccount
		if err := db.First(&row, id).Error; err != nil {
			return nil, fmt.Errorf("bank account %d not found", id)
		}
		if row.Archived {
			return nil, fmt.Errorf("bank account %q is archived", row.Label)
		}
		out = append(out, row.ToDetails())
	}
	return out, nil
}

func normalize(input Input) (BankAccount, error) {
	row := BankAccount{
		Label:         strings.TrimSpace(input.Label),
		AccountName:   strings.TrimSpace(input.AccountName),
		BankName:      strings.TrimSpace(input.BankName),
		Currency:      strings.ToUpper(strings.TrimSpace(input.Currency)),
		AccountNumber: strings.TrimSpace(input.AccountNumber),
		IBAN:          strings.ToUpper(strings.Join(strings.Fields(input.IBAN), "")),
		SwiftCode:     strings.ToUpper(strings.Join(strings.Fields(input.SwiftCode), "")),
		IsDefault:     input.IsDefault && !input.Archived,
		Archived:      input.Archived,
	}
	switch {
	case row.AccountName == "":
		return row, errors.New("account name is required")
	case row.BankName == "":
		return row, errors.New("bank name is required")
	case len(row.Currency) != 3:
		return row, errors.New("currency must be a 3-letter code such as AED or USD")
	case row.AccountNumber == "" && row.IBAN == "":
		return row, errors.New("enter an account number or an IBAN")
	case row.SwiftCode != "" && len(row.SwiftCode) != 8 && len(row.SwiftCode) != 11:
		return row, errors.New("SWIFT / BIC code must be 8 or 11 characters")
	}
	if row.Label == "" {
		row.Label = row.BankName + " – " + row.Currency
	}
	return row, nil
}

func clearOtherDefaults(tx *gorm.DB, row *BankAccount) error {
	if !row.IsDefault {
		return nil
	}
	return tx.Model(&BankAccount{}).
		Where("currency = ? AND id <> ? AND is_default = ?", row.Currency, row.ID, true).
		Update("is_default", false).Error
}

// writeAudit records before/after values: changed bank details are a common invoice-fraud vector.
func writeAudit(tx *gorm.DB, actorID *uint, action, method, path string, before, after *BankAccount) error {
	payload, _ := json.Marshal(map[string]any{"before": before, "after": after})
	return tx.Create(&models.AuditLog{
		UserID:  actorID,
		Action:  action,
		Method:  method,
		Path:    path,
		Status:  200,
		Payload: string(payload),
	}).Error
}
