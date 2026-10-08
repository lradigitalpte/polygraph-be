package accounting

import (
	"testing"
)

func TestAllocatePaymentVAT_FullPayment(t *testing.T) {
	ex, vat := AllocatePaymentVAT(105, 100, 5, 105)
	if ex != 100 || vat != 5 {
		t.Fatalf("full payment: got ex=%v vat=%v, want 100, 5", ex, vat)
	}
}

func TestAllocatePaymentVAT_HalfPayment(t *testing.T) {
	ex, vat := AllocatePaymentVAT(53, 100, 5, 105)
	if ex != 51 || vat != 3 {
		t.Fatalf("half payment: got ex=%v vat=%v, want 51, 3", ex, vat)
	}
}

func TestAllocatePaymentVAT_ZeroInvoice(t *testing.T) {
	ex, vat := AllocatePaymentVAT(50, 100, 5, 0)
	if ex != 0 || vat != 0 {
		t.Fatalf("zero invoice: got ex=%v vat=%v", ex, vat)
	}
}

func TestNormalizeExpenseAmounts_FromExVatAndRate(t *testing.T) {
	ex, rate, vat, inc := normalizeExpenseAmounts(expenseAmountInput{
		AmountExVat: 100,
		VatRate:     5,
	})
	if ex != 100 || rate != 5 || vat != 5 || inc != 105 {
		t.Fatalf("got ex=%v rate=%v vat=%v inc=%v", ex, rate, vat, inc)
	}
}
