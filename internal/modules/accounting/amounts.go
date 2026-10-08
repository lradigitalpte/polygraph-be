package accounting

import "my-app/internal/money"

type expenseAmountInput struct {
	AmountExVat  float64
	VatRate      float64
	VatAmount    *float64
	AmountIncVat float64
}

func normalizeExpenseAmounts(in expenseAmountInput) (exVat, rate, vat, incVat float64) {
	exVat = money.CeilWhole(in.AmountExVat)
	rate = in.VatRate
	if rate < 0 {
		rate = 0
	}

	if in.VatAmount != nil {
		vat = money.CeilWhole(*in.VatAmount)
	} else if rate > 0 && exVat > 0 {
		vat = money.CeilWhole(exVat * rate / 100)
	}

	if in.AmountIncVat > 0 {
		incVat = money.CeilWhole(in.AmountIncVat)
	} else {
		incVat = exVat + vat
	}

	if exVat <= 0 && incVat > 0 && vat > 0 {
		exVat = incVat - vat
		if exVat < 0 {
			exVat = 0
		}
	}

	return exVat, rate, vat, incVat
}

// AllocatePaymentVAT splits a payment gross amount into ex-VAT and VAT portions.
func AllocatePaymentVAT(paymentGross, exVatTotal, vatTotal, invoiceTotal float64) (netPortion, vatPortion float64) {
	paymentGross = money.CeilWhole(paymentGross)
	if paymentGross <= 0 || invoiceTotal <= 0 {
		return 0, 0
	}
	exVatTotal = money.CeilWhole(exVatTotal)
	vatTotal = money.CeilWhole(vatTotal)
	invoiceTotal = money.CeilWhole(invoiceTotal)
	if invoiceTotal <= 0 {
		return 0, 0
	}
	netPortion = money.CeilWhole(paymentGross * exVatTotal / invoiceTotal)
	vatPortion = money.CeilWhole(paymentGross * vatTotal / invoiceTotal)
	return netPortion, vatPortion
}
