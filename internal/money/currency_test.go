package money

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvert_USDToAED(t *testing.T) {
	rates := Rates{Currency: "AED", UsdAedRate: 3.6725}
	got := Round2(Convert(450, "USD", "AED", rates))
	assert.Equal(t, 1652.63, got)
}

func TestConvert_AEDToUSD(t *testing.T) {
	rates := Rates{Currency: "AED", UsdAedRate: 3.6725}
	got := Round2(Convert(1652.63, "AED", "USD", rates))
	assert.InDelta(t, 450, got, 0.02)
}

func TestFromUSD(t *testing.T) {
	rates := Rates{Currency: "AED", UsdAedRate: 3.6725}
	assert.Equal(t, 550.0, Round2(Convert(550, "USD", "USD", rates)))
	assert.Equal(t, 2019.88, Round2(FromUSD(550, rates)))
}
