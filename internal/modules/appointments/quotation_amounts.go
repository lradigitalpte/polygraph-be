package appointments

import "my-app/internal/money"

// InvoiceAmounts holds ex-VAT (after discount), VAT, and total incl. VAT for a quotation.
type InvoiceAmounts struct {
	ExVAT float64 `json:"ex_vat"`
	VAT   float64 `json:"vat"`
	Total float64 `json:"total"`
}

// QuotationHasVAT reports whether the invoice uses VAT (included on VAT returns).
func QuotationHasVAT(q *Quotation) bool {
	if q == nil {
		return false
	}
	return q.VatRate > 0 || money.CeilWhole(q.VatAmount) > 0
}

// QuotationTotalDue returns the invoice total incl. VAT (matches dashboard and PDF).
func QuotationTotalDue(q *Quotation) float64 {
	if q == nil {
		return 0
	}
	return computeInvoiceTotals(q).Total
}

// quotationPaymentStatusFromAmounts maps collected vs total due to invoice status.
func quotationPaymentStatusFromAmounts(collected, totalDue float64) string {
	collected = money.CeilWhole(collected)
	totalDue = money.CeilWhole(totalDue)
	if totalDue <= 0 {
		return "Pending"
	}
	if collected >= totalDue {
		return "Completed"
	}
	if collected > 0 {
		return "Partial"
	}
	return "Pending"
}

// QuotationInvoiceAmounts derives display totals using the same rules as invoice PDFs.
func QuotationInvoiceAmounts(q *Quotation) InvoiceAmounts {
	t := computeInvoiceTotals(q)
	ex := t.Subtotal - t.Discount
	if ex < 0 {
		ex = 0
	}
	return InvoiceAmounts{
		ExVAT: ex,
		VAT:   t.VAT,
		Total: t.Total,
	}
}

// QuotationPaymentHistory parses the payment_history JSON column.
func QuotationPaymentHistory(raw string) []QuotationPaymentEntry {
	return parseQuotationPaymentHistory(raw)
}
