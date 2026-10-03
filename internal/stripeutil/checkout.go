package stripeutil

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"

	"my-app/internal/money"
)

// CheckoutResult is the hosted Checkout URL and related Stripe IDs.
type CheckoutResult struct {
	SessionID string
	URL       string
}

// Configured reports whether Stripe secret key is available.
func Configured() bool {
	return strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")) != ""
}

func initStripe() error {
	key := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if key == "" {
		return errors.New("STRIPE_SECRET_KEY is not configured")
	}
	stripe.Key = key
	return nil
}

func frontendBaseURL() string {
	if u := strings.TrimSpace(os.Getenv("FRONTEND_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := strings.TrimSpace(os.Getenv("APP_PUBLIC_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:3000"
}

// ToStripeAmount converts a major-unit amount (e.g. 150.50) to Stripe's smallest unit.
func ToStripeAmount(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

// FromStripeAmount converts Stripe's smallest unit back to major units.
func FromStripeAmount(cents int64) float64 {
	return float64(cents) / 100.0
}

// CreateCheckoutSessionParams are inputs for a one-time invoice payment link.
type CreateCheckoutSessionParams struct {
	QuotationID   uint
	AppointmentID *uint
	Code          string
	Title         string
	CustomerEmail string
	Currency      string
	ChargeAmount  float64
	// PassProcessingFeeToCustomer adds a separate Checkout line item (estimated card fee).
	PassProcessingFeeToCustomer bool
	ProcessingFeePercent          float64
	ProcessingFeeFixed            float64
}

// CreateCheckoutSession creates a one-time Stripe Checkout Session for an invoice balance or deposit.
func CreateCheckoutSession(p CreateCheckoutSessionParams) (*CheckoutResult, error) {
	if err := initStripe(); err != nil {
		return nil, err
	}
	if p.ChargeAmount <= 0 {
		return nil, errors.New("charge amount must be greater than zero")
	}
	currency := strings.ToLower(strings.TrimSpace(p.Currency))
	if currency == "" {
		currency = "usd"
	}
	netAmount := money.CeilWhole(p.ChargeAmount)
	unitAmount := ToStripeAmount(netAmount)
	if unitAmount < 1 {
		return nil, errors.New("charge amount is too small")
	}

	var processingFee float64
	var grossAmount float64 = netAmount
	if p.PassProcessingFeeToCustomer {
		grossAmount, processingFee = GrossChargeWithProcessingFee(
			netAmount,
			p.ProcessingFeePercent,
			p.ProcessingFeeFixed,
		)
	}

	name := strings.TrimSpace(p.Title)
	if name == "" {
		name = "Polygraph invoice"
	}
	desc := strings.TrimSpace(p.Code)
	if desc == "" {
		desc = fmt.Sprintf("Quotation #%d", p.QuotationID)
	}

	meta := map[string]string{
		"quotation_id":  strconv.FormatUint(uint64(p.QuotationID), 10),
		"charge_amount": fmt.Sprintf("%.2f", netAmount),
	}
	if processingFee > 0 {
		meta["processing_fee"] = fmt.Sprintf("%.2f", processingFee)
		meta["total_charged"] = fmt.Sprintf("%.2f", grossAmount)
	}
	if p.AppointmentID != nil {
		meta["appointment_id"] = strconv.FormatUint(uint64(*p.AppointmentID), 10)
	}

	lineItems := []*stripe.CheckoutSessionLineItemParams{
		{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String(currency),
				UnitAmount: stripe.Int64(unitAmount),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String(name),
					Description: stripe.String(desc),
				},
			},
		},
	}
	if processingFee > 0 {
		feeCents := ToStripeAmount(processingFee)
		if feeCents >= 1 {
			lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
				Quantity: stripe.Int64(1),
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String(currency),
					UnitAmount: stripe.Int64(feeCents),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String("Processing fee"),
						Description: stripe.String("Estimated online processing fee (non-refundable)"),
					},
				},
			})
		}
	}

	base := frontendBaseURL()
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(base + "/pay/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(base + "/pay/cancel"),
		LineItems:  lineItems,
		Metadata: meta,
		PaymentIntentData: &stripe.CheckoutSessionPaymentIntentDataParams{
			Metadata: meta,
		},
	}
	if email := strings.TrimSpace(p.CustomerEmail); email != "" && strings.Contains(email, "@") {
		params.CustomerEmail = stripe.String(email)
	}

	sess, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe checkout session: %w", err)
	}
	if sess.URL == "" {
		return nil, errors.New("stripe checkout session missing URL")
	}
	return &CheckoutResult{SessionID: sess.ID, URL: sess.URL}, nil
}
