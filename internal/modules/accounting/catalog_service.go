package accounting

import (
	"errors"
	"strings"
	"unicode"
)

type PaymentMethodInput struct {
	Label     string `json:"label"`
	Type      string `json:"type"`
	LastFour  string `json:"last_four"`
	BankName  string `json:"bank_name"`
	IsDefault bool   `json:"is_default"`
}

type PurchaseItemInput struct {
	Name               string  `json:"name"`
	Description        string  `json:"description"`
	Category           string  `json:"category"`
	DefaultAmountExVat float64 `json:"default_amount_ex_vat"`
	VatMode            string  `json:"vat_mode"`
	VatRate            float64 `json:"vat_rate"`
	VatAmountFixed     float64 `json:"vat_amount_fixed"`
	Active             *bool   `json:"active"`
}

func normalizeLastFour(raw string) string {
	var digits strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	s := digits.String()
	if len(s) > 4 {
		s = s[len(s)-4:]
	}
	return s
}

func normalizePaymentType(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	switch t {
	case "credit_card", "credit", "card":
		return "credit_card"
	case "bank", "bank_transfer", "transfer":
		return "bank"
	case "cash":
		return "cash"
	default:
		if t == "" {
			return "other"
		}
		return t
	}
}

func (s *Service) ListPaymentMethods() ([]ExpensePaymentMethod, error) {
	items := make([]ExpensePaymentMethod, 0)
	err := s.db.Order("is_default DESC, label ASC").Find(&items).Error
	return items, err
}

func (s *Service) CreatePaymentMethod(input PaymentMethodInput) (*ExpensePaymentMethod, error) {
	label := strings.TrimSpace(input.Label)
	if label == "" {
		return nil, errors.New("label is required")
	}
	item := ExpensePaymentMethod{
		Label:     label,
		Type:      normalizePaymentType(input.Type),
		LastFour:  normalizeLastFour(input.LastFour),
		BankName:  strings.TrimSpace(input.BankName),
		IsDefault: input.IsDefault,
	}
	if err := s.db.Create(&item).Error; err != nil {
		return nil, err
	}
	if item.IsDefault {
		_ = s.db.Model(&ExpensePaymentMethod{}).Where("id != ?", item.ID).Update("is_default", false).Error
	}
	return &item, nil
}

func (s *Service) UpdatePaymentMethod(id uint, input PaymentMethodInput) (*ExpensePaymentMethod, error) {
	var item ExpensePaymentMethod
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	if label := strings.TrimSpace(input.Label); label != "" {
		item.Label = label
	}
	if input.Type != "" {
		item.Type = normalizePaymentType(input.Type)
	}
	if input.LastFour != "" {
		item.LastFour = normalizeLastFour(input.LastFour)
	}
	item.BankName = strings.TrimSpace(input.BankName)
	item.IsDefault = input.IsDefault
	if err := s.db.Save(&item).Error; err != nil {
		return nil, err
	}
	if item.IsDefault {
		_ = s.db.Model(&ExpensePaymentMethod{}).Where("id != ?", item.ID).Update("is_default", false).Error
	}
	return &item, nil
}

func (s *Service) DeletePaymentMethod(id uint) error {
	return s.db.Delete(&ExpensePaymentMethod{}, id).Error
}

func (s *Service) ListPurchaseItems(search string, includeInactive bool) ([]ExpensePurchaseItem, error) {
	items := make([]ExpensePurchaseItem, 0)
	q := s.db.Model(&ExpensePurchaseItem{})
	if !includeInactive {
		q = q.Where("active = ?", true)
	}
	search = strings.TrimSpace(strings.ToLower(search))
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ? OR LOWER(category) LIKE ?", like, like, like)
	}
	err := q.Order("name ASC").Find(&items).Error
	return items, err
}

func (s *Service) CreatePurchaseItem(input PurchaseItemInput) (*ExpensePurchaseItem, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	mode := strings.TrimSpace(input.VatMode)
	if mode == "" {
		mode = "rate"
	}
	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = "General"
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	item := ExpensePurchaseItem{
		Name:               name,
		Description:        strings.TrimSpace(input.Description),
		Category:           category,
		DefaultAmountExVat: input.DefaultAmountExVat,
		VatMode:            mode,
		VatRate:            input.VatRate,
		VatAmountFixed:     input.VatAmountFixed,
		Active:             active,
	}
	if err := s.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) UpdatePurchaseItem(id uint, input PurchaseItemInput) (*ExpensePurchaseItem, error) {
	var item ExpensePurchaseItem
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	if name := strings.TrimSpace(input.Name); name != "" {
		item.Name = name
	}
	item.Description = strings.TrimSpace(input.Description)
	if cat := strings.TrimSpace(input.Category); cat != "" {
		item.Category = cat
	}
	item.DefaultAmountExVat = input.DefaultAmountExVat
	if input.VatMode != "" {
		item.VatMode = strings.TrimSpace(input.VatMode)
	}
	item.VatRate = input.VatRate
	item.VatAmountFixed = input.VatAmountFixed
	if input.Active != nil {
		item.Active = *input.Active
	}
	if err := s.db.Save(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) DeletePurchaseItem(id uint) error {
	return s.db.Delete(&ExpensePurchaseItem{}, id).Error
}

func (s *Service) maybeSavePaymentMethod(input CreateExpenseInput) (*uint, error) {
	if !input.SavePaymentMethod {
		return nil, nil
	}
	label := strings.TrimSpace(input.PaymentLabel)
	if label == "" {
		label = strings.TrimSpace(input.PaymentMethodLabel)
	}
	lastFour := normalizeLastFour(input.PaymentLastFour)
	payType := normalizePaymentType(input.PaymentType)
	if label == "" && lastFour == "" {
		return nil, nil
	}
	if label == "" {
		if payType == "credit_card" {
			label = "Card •••• " + lastFour
		} else if payType == "bank" {
			label = "Bank •••• " + lastFour
		} else {
			label = "Payment method"
		}
	}
	created, err := s.CreatePaymentMethod(PaymentMethodInput{
		Label:    label,
		Type:     payType,
		LastFour: lastFour,
		BankName: strings.TrimSpace(input.PaymentBankName),
	})
	if err != nil {
		return nil, err
	}
	return &created.ID, nil
}

func (s *Service) applyPaymentMethodToExpense(item *Expense, input CreateExpenseInput, newMethodID *uint) {
	if newMethodID != nil {
		item.PaymentMethodID = newMethodID
	}
	if input.PaymentMethodID != nil && *input.PaymentMethodID > 0 {
		item.PaymentMethodID = input.PaymentMethodID
		var pm ExpensePaymentMethod
		if err := s.db.First(&pm, *item.PaymentMethodID).Error; err == nil {
			item.PaymentType = pm.Type
			item.PaymentLabel = pm.Label
			item.PaymentLastFour = pm.LastFour
			return
		}
	}
	item.PaymentType = normalizePaymentType(input.PaymentType)
	item.PaymentLabel = strings.TrimSpace(input.PaymentLabel)
	if item.PaymentLabel == "" {
		item.PaymentLabel = strings.TrimSpace(input.PaymentMethodLabel)
	}
	item.PaymentLastFour = normalizeLastFour(input.PaymentLastFour)
}
