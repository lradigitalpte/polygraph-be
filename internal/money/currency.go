package money

import (
	"math"
	"strings"

	"gorm.io/gorm"
)

// Rates holds organization billing currency and USD cross-rates.
type Rates struct {
	Currency   string
	UsdAedRate float64
	UsdGbpRate float64
	UsdEurRate float64
}

// LoadRates reads organization billing settings (singleton id = 1).
func LoadRates(db *gorm.DB) Rates {
	rates := Rates{
		Currency:   "USD",
		UsdAedRate: 3.6725,
		UsdGbpRate: 0.7850,
		UsdEurRate: 0.9250,
	}
	var row struct {
		Currency   string  `gorm:"column:currency"`
		UsdAedRate float64 `gorm:"column:usd_aed_rate"`
		UsdGbpRate float64 `gorm:"column:usd_gbp_rate"`
		UsdEurRate float64 `gorm:"column:usd_eur_rate"`
	}
	if err := db.Table("organization_settings").Select("currency", "usd_aed_rate", "usd_gbp_rate", "usd_eur_rate").Where("id = ?", 1).First(&row).Error; err != nil {
		return rates
	}
	if c := strings.ToUpper(strings.TrimSpace(row.Currency)); c != "" {
		rates.Currency = c
	}
	if row.UsdAedRate > 0 {
		rates.UsdAedRate = row.UsdAedRate
	}
	if row.UsdGbpRate > 0 {
		rates.UsdGbpRate = row.UsdGbpRate
	}
	if row.UsdEurRate > 0 {
		rates.UsdEurRate = row.UsdEurRate
	}
	return rates
}

func rateMap(r Rates) map[string]float64 {
	return map[string]float64{
		"USD": 1,
		"AED": r.UsdAedRate,
		"GBP": r.UsdGbpRate,
		"EUR": r.UsdEurRate,
	}
}

// Convert mirrors the frontend convertCurrency helper.
func Convert(amount float64, from, to string, r Rates) float64 {
	cleanFrom := strings.ToUpper(strings.TrimSpace(from))
	if cleanFrom == "" {
		cleanFrom = "USD"
	}
	cleanTo := strings.ToUpper(strings.TrimSpace(to))
	if cleanTo == "" {
		cleanTo = "USD"
	}
	if cleanFrom == cleanTo {
		return amount
	}

	rates := rateMap(r)
	rateFrom := rates[cleanFrom]
	if rateFrom == 0 {
		rateFrom = 1
	}
	rateTo := rates[cleanTo]
	if rateTo == 0 {
		rateTo = 1
	}

	amountInUSD := amount / rateFrom
	return amountInUSD * rateTo
}

// FromUSD converts a USD catalog amount into the organization currency.
func FromUSD(amount float64, r Rates) float64 {
	return Convert(amount, "USD", r.Currency, r)
}

// Round2 rounds to two decimal places for monetary values.
func Round2(amount float64) float64 {
	return math.Round(amount*100) / 100
}

// ApproxEqual compares monetary values within one cent.
func ApproxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.005
}
