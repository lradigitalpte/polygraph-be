package stripeutil

import "my-app/internal/money"

// GrossChargeWithProcessingFee returns the total the customer pays so that `netAmount`
// is credited to the invoice after an estimated card fee (percent + fixed in same currency).
func GrossChargeWithProcessingFee(netAmount, percent, fixed float64) (gross, fee float64) {
	netAmount = money.CeilWhole(netAmount)
	if netAmount <= 0 {
		return 0, 0
	}
	if percent < 0 {
		percent = 0
	}
	if fixed < 0 {
		fixed = 0
	}
	fixed = money.CeilWhole(fixed)
	if percent == 0 && fixed == 0 {
		return netAmount, 0
	}
	rate := percent / 100.0
	if rate >= 1 {
		return netAmount, 0
	}
	gross = money.CeilWhole((netAmount + fixed) / (1 - rate))
	fee = gross - netAmount
	if fee < 0 {
		fee = 0
	}
	return gross, fee
}
