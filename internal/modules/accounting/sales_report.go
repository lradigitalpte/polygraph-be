package accounting

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"my-app/internal/modules/appointments"
	"my-app/internal/money"
)

type SalesReportSummary struct {
	TotalGrossInclVat float64 `json:"total_gross_incl_vat"`
	TotalExVat          float64 `json:"total_ex_vat"`
	TotalVat            float64 `json:"total_vat"`
	PaymentLineCount    int     `json:"payment_line_count"`
	WithVatLineCount    int     `json:"with_vat_line_count"`
	WithoutVatLineCount int     `json:"without_vat_line_count"`
	Currency            string  `json:"currency"`
}

type SalesReportLine struct {
	QuotationID  uint      `json:"quotation_id"`
	InvoiceCode  string    `json:"invoice_code"`
	ClientName   string    `json:"client_name"`
	PaidAt       time.Time `json:"paid_at"`
	Method       string    `json:"method"`
	HasVat       bool      `json:"has_vat"`
	VatRate      float64   `json:"vat_rate"`
	GrossPaid    float64   `json:"gross_paid"`
	ExVatPortion float64   `json:"ex_vat_portion"`
	VatPortion   float64   `json:"vat_portion"`
	Currency     string    `json:"currency"`
}

type SalesReport struct {
	From    time.Time         `json:"from"`
	To      time.Time         `json:"to"`
	Summary SalesReportSummary `json:"summary"`
	Lines   []SalesReportLine  `json:"lines"`
}

func (s *Service) BuildSalesReport(from, to time.Time) (*SalesReport, error) {
	from = startOfUTCDay(from)
	to = endOfUTCDay(to)
	if to.Before(from) {
		return nil, errors.New("invalid date range")
	}

	currency := money.LoadRates(s.db).Currency

	var quotes []appointments.Quotation
	err := s.db.
		Preload("Client").
		Where("payment_history IS NOT NULL AND payment_history != '' AND payment_history != '[]'").
		Find(&quotes).Error
	if err != nil {
		return nil, err
	}

	var lines []SalesReportLine
	var sumGross, sumEx, sumVat float64
	var withVat, withoutVat int

	for i := range quotes {
		q := &quotes[i]
		amounts := appointments.QuotationInvoiceAmounts(q)
		if amounts.Total <= 0 {
			continue
		}
		hasVat := appointments.QuotationHasVAT(q)

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

			var exPortion, vatPortion float64
			if hasVat {
				exPortion, vatPortion = AllocatePaymentVAT(gross, amounts.ExVAT, amounts.VAT, amounts.Total)
			} else {
				exPortion = gross
				vatPortion = 0
			}

			lines = append(lines, SalesReportLine{
				QuotationID:  q.ID,
				InvoiceCode:  code,
				ClientName:   clientName,
				PaidAt:       paidAt,
				Method:       strings.TrimSpace(entry.Method),
				HasVat:       hasVat,
				VatRate:      q.VatRate,
				GrossPaid:    gross,
				ExVatPortion: exPortion,
				VatPortion:   vatPortion,
				Currency:     cur,
			})

			sumGross += gross
			sumEx += exPortion
			sumVat += vatPortion
			if hasVat {
				withVat++
			} else {
				withoutVat++
			}
		}
	}

	return &SalesReport{
		From: from,
		To:   to,
		Lines: lines,
		Summary: SalesReportSummary{
			TotalGrossInclVat: sumGross,
			TotalExVat:          sumEx,
			TotalVat:              sumVat,
			PaymentLineCount:      len(lines),
			WithVatLineCount:      withVat,
			WithoutVatLineCount:   withoutVat,
			Currency:              currency,
		},
	}, nil
}
