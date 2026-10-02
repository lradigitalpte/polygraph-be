package stripeutil

import "math"

// GrossChargeWithProcessingFee returns the total the customer pays so that `netAmount`
// is credited to the invoice after an estimated card fee (percent + fixed in same currency).
func GrossChargeWithProcessingFee(netAmount, percent, fixed float64) (gross, fee float64) {
	if netAmount <= 0 {
		return 0, 0
	}
	if percent < 0 {
		percent = 0
	}
	if fixed < 0 {
		fixed = 0
	}
	if percent == 0 && fixed == 0 {
		return netAmount, 0
	}
	rate := percent / 100.0
	if rate >= 1 {
		return netAmount, 0
	}
	gross = (netAmount + fixed) / (1 - rate)
	gross = math.Round(gross*100) / 100
	fee = gross - netAmount
	if fee < 0 {
		fee = 0
	}
	fee = math.Round(fee*100) / 100
	return gross, fee
}
