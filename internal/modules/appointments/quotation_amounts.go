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
