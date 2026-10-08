package accounting

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"my-app/internal/database"
	"my-app/internal/modules/appointments"
	"my-app/internal/money"
)

type Service struct {
	db *gorm.DB
}

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

type CreateExpenseInput struct {
	ExpenseDate  time.Time `json:"expense_date"`
	Vendor       string    `json:"vendor"`
	Description  string    `json:"description"`
	Category     string    `json:"category"`
	AmountExVat  float64   `json:"amount_ex_vat"`
	VatRate      float64   `json:"vat_rate"`
	VatAmount    *float64  `json:"vat_amount"`
	AmountIncVat float64   `json:"amount_inc_vat"`
	Currency     string    `json:"currency"`
	ReceiptRef   string    `json:"receipt_ref"`
}

type UpdateExpenseInput struct {
	ExpenseDate  *time.Time `json:"expense_date"`
	Vendor       *string    `json:"vendor"`
	Description  *string    `json:"description"`
	Category     *string    `json:"category"`
	AmountExVat  *float64   `json:"amount_ex_vat"`
	VatRate      *float64   `json:"vat_rate"`
	VatAmount    *float64   `json:"vat_amount"`
	AmountIncVat *float64   `json:"amount_inc_vat"`
	Currency     *string    `json:"currency"`
	ReceiptRef   *string    `json:"receipt_ref"`
}

func (s *Service) ListExpenses(from, to *time.Time) ([]Expense, error) {
	var items []Expense
	q := s.db.Model(&Expense{})
	if from != nil {
		q = q.Where("expense_date >= ?", *from)
	}
	if to != nil {
		q = q.Where("expense_date <= ?", *to)
	}
	err := q.Order("expense_date DESC, id DESC").Find(&items).Error
	return items, err
}

func (s *Service) GetExpense(id uint) (*Expense, error) {
	var item Expense
	if err := s.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) CreateExpense(input CreateExpenseInput, createdBy *uint) (*Expense, error) {
	if input.ExpenseDate.IsZero() {
		return nil, errors.New("expense_date is required")
	}
	ex, rate, vat, inc := normalizeExpenseAmounts(expenseAmountInput{
		AmountExVat:  input.AmountExVat,
		VatRate:      input.VatRate,
		VatAmount:    input.VatAmount,
		AmountIncVat: input.AmountIncVat,
	})
	if inc <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency == "" {
		currency = money.LoadRates(s.db).Currency
	}

	item := Expense{
		ExpenseDate:     input.ExpenseDate.UTC(),
		Vendor:          strings.TrimSpace(input.Vendor),
		Description:     strings.TrimSpace(input.Description),
		Category:        strings.TrimSpace(input.Category),
		AmountExVat:     ex,
		VatRate:         rate,
		VatAmount:       vat,
		AmountIncVat:    inc,
		Currency:        currency,
		ReceiptRef:      strings.TrimSpace(input.ReceiptRef),
		CreatedByUserID: createdBy,
	}
	if item.Category == "" {
		item.Category = "General"
	}

	if err := s.db.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) UpdateExpense(id uint, input UpdateExpenseInput) (*Expense, error) {
	item, err := s.GetExpense(id)
	if err != nil {
		return nil, err
	}

	if input.ExpenseDate != nil && !input.ExpenseDate.IsZero() {
		item.ExpenseDate = input.ExpenseDate.UTC()
	}
	if input.Vendor != nil {
		item.Vendor = strings.TrimSpace(*input.Vendor)
	}
	if input.Description != nil {
		item.Description = strings.TrimSpace(*input.Description)
	}
	if input.Category != nil {
		c := strings.TrimSpace(*input.Category)
		if c != "" {
			item.Category = c
		}
	}
	if input.ReceiptRef != nil {
		item.ReceiptRef = strings.TrimSpace(*input.ReceiptRef)
	}
	if input.Currency != nil {
		c := strings.ToUpper(strings.TrimSpace(*input.Currency))
		if c != "" {
			item.Currency = c
		}
	}

	amountsTouched := input.AmountExVat != nil || input.VatRate != nil || input.VatAmount != nil || input.AmountIncVat != nil
	if amountsTouched {
		exIn := item.AmountExVat
		if input.AmountExVat != nil {
			exIn = *input.AmountExVat
		}
		rateIn := item.VatRate
		if input.VatRate != nil {
			rateIn = *input.VatRate
		}
		var vatPtr *float64
		if input.VatAmount != nil {
			vatPtr = input.VatAmount
		} else {
			v := item.VatAmount
			vatPtr = &v
		}
		incIn := item.AmountIncVat
		if input.AmountIncVat != nil {
			incIn = *input.AmountIncVat
		}
		ex, rate, vat, inc := normalizeExpenseAmounts(expenseAmountInput{
			AmountExVat:  exIn,
			VatRate:      rateIn,
			VatAmount:    vatPtr,
			AmountIncVat: incIn,
		})
		if inc <= 0 {
			return nil, errors.New("amount must be greater than zero")
		}
		item.AmountExVat = ex
		item.VatRate = rate
		item.VatAmount = vat
		item.AmountIncVat = inc
	}

	if err := s.db.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) DeleteExpense(id uint) error {
	return s.db.Delete(&Expense{}, id).Error
}

type VatReturnSummary struct {
	TaxableSuppliesExVat float64 `json:"taxable_supplies_ex_vat"`
	OutputVat            float64 `json:"output_vat"`
	InputVat             float64 `json:"input_vat"`
	NetVatPayable        float64 `json:"net_vat_payable"`
	Currency             string  `json:"currency"`
}

type VatReturnOutputLine struct {
	QuotationID   uint      `json:"quotation_id"`
	InvoiceCode   string    `json:"invoice_code"`
	ClientName    string    `json:"client_name"`
	PaidAt        time.Time `json:"paid_at"`
	Method        string    `json:"method"`
	GrossPaid     float64   `json:"gross_paid"`
	ExVatPortion  float64   `json:"ex_vat_portion"`
	VatPortion    float64   `json:"vat_portion"`
	Currency      string    `json:"currency"`
}

type VatReturnInputLine struct {
	ExpenseID    uint      `json:"expense_id"`
	ExpenseDate  time.Time `json:"expense_date"`
	Vendor       string    `json:"vendor"`
	Category     string    `json:"category"`
	Description  string    `json:"description"`
	AmountExVat  float64   `json:"amount_ex_vat"`
	VatRate      float64   `json:"vat_rate"`
	VatAmount    float64   `json:"vat_amount"`
	AmountIncVat float64   `json:"amount_inc_vat"`
	Currency     string    `json:"currency"`
}

type VatReturnReport struct {
	From         time.Time            `json:"from"`
	To           time.Time            `json:"to"`
	Summary      VatReturnSummary     `json:"summary"`
	OutputLines  []VatReturnOutputLine `json:"output_lines"`
	InputLines   []VatReturnInputLine  `json:"input_lines"`
}

func (s *Service) BuildVatReturn(from, to time.Time) (*VatReturnReport, error) {
	from = startOfUTCDay(from)
	to = endOfUTCDay(to)
	if to.Before(from) {
		return nil, errors.New("invalid date range")
	}

	currency := money.LoadRates(s.db).Currency

	var quotes []appointments.Quotation
	err := s.db.
		Preload("Client").
		Where("vat_rate > 0 OR vat_amount > 0").
		Find(&quotes).Error
	if err != nil {
		return nil, err
	}

	outputLines := make([]VatReturnOutputLine, 0)
	var taxableEx float64
	var outputVat float64

	for i := range quotes {
		q := &quotes[i]
		if !appointments.QuotationHasVAT(q) {
			continue
		}
		amounts := appointments.QuotationInvoiceAmounts(q)
		if amounts.Total <= 0 {
			continue
		}

		cur := strings.ToUpper(strings.TrimSpace(q.Currency))
		if cur == "" {
			cur = currency
		}

		clientName := strings.TrimSpace(q.Client.Name)
		code := strings.TrimSpace(q.Code)
		if code == "" {
			code = fmt.Sprintf("INV-%d", q.ID)
		}

		history := appointments.QuotationPaymentHistory(q.PaymentHistoryJSON)
		for _, entry := range history {
			paidAt := entry.PaidAt.UTC()
			if paidAt.Before(from) || paidAt.After(to) {
				continue
			}
			gross := money.CeilWhole(entry.Amount)
			if gross <= 0 {
				continue
			}
			exPortion, vatPortion := AllocatePaymentVAT(gross, amounts.ExVAT, amounts.VAT, amounts.Total)
			outputLines = append(outputLines, VatReturnOutputLine{
				QuotationID:  q.ID,
				InvoiceCode:  code,
				ClientName:   clientName,
				PaidAt:       paidAt,
				Method:       strings.TrimSpace(entry.Method),
				GrossPaid:    gross,
				ExVatPortion: exPortion,
				VatPortion:   vatPortion,
				Currency:     cur,
			})
			taxableEx += exPortion
			outputVat += vatPortion
		}
	}

	expenses, err := s.ListExpenses(&from, &to)
	if err != nil {
		return nil, err
	}

	inputLines := make([]VatReturnInputLine, 0)
	var inputVat float64
	for _, e := range expenses {
		inputLines = append(inputLines, VatReturnInputLine{
			ExpenseID:    e.ID,
			ExpenseDate:  e.ExpenseDate,
			Vendor:       e.Vendor,
			Category:     e.Category,
			Description:  e.Description,
			AmountExVat:  e.AmountExVat,
			VatRate:      e.VatRate,
			VatAmount:    e.VatAmount,
			AmountIncVat: e.AmountIncVat,
			Currency:     e.Currency,
		})
		inputVat += e.VatAmount
	}

	report := &VatReturnReport{
		From:        from,
		To:          to,
		OutputLines: outputLines,
		InputLines:  inputLines,
		Summary: VatReturnSummary{
			TaxableSuppliesExVat: taxableEx,
			OutputVat:            outputVat,
			InputVat:             inputVat,
			NetVatPayable:        outputVat - inputVat,
			Currency:             currency,
		},
	}
	return report, nil
}

func startOfUTCDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func endOfUTCDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, time.UTC)
}
