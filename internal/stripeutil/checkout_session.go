package stripeutil

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
)

// CheckoutPaymentDetails is parsed from a paid Checkout Session.
type CheckoutPaymentDetails struct {
	QuotationID     uint
	Amount          float64
	SessionID       string
	PaymentIntentID string
}

// RetrieveCheckoutSession loads the latest session state from Stripe (including metadata).
func RetrieveCheckoutSession(sessionID string) (*stripe.CheckoutSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	if err := initStripe(); err != nil {
		return nil, err
	}
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("payment_intent")
	return session.Get(sessionID, params)
}

// CheckoutSessionIsPaid reports whether the session represents a successful charge.
func CheckoutSessionIsPaid(sess *stripe.CheckoutSession) bool {
	if sess == nil {
		return false
	}
	if sess.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid ||
		sess.PaymentStatus == stripe.CheckoutSessionPaymentStatusNoPaymentRequired {
		return true
	}
	if sess.Status != stripe.CheckoutSessionStatusComplete {
		return false
	}
	if sess.AmountTotal > 0 {
		return true
	}
	if amount, ok := metaChargeAmount(sess.Metadata); ok && amount > 0 {
		return true
	}
	return false
}

// ParseCheckoutSessionPayment extracts quotation payment fields from a paid session.
func ParseCheckoutSessionPayment(sess *stripe.CheckoutSession) (*CheckoutPaymentDetails, error) {
	if sess == nil || strings.TrimSpace(sess.ID) == "" {
		return nil, errors.New("missing checkout session")
	}
	if !CheckoutSessionIsPaid(sess) {
		return nil, fmt.Errorf(
			"checkout session is not paid (status=%q payment_status=%q)",
			sess.Status,
			sess.PaymentStatus,
		)
	}

	quotationID, err := parseUintMeta(sess.Metadata, "quotation_id")
	if err != nil || quotationID == 0 {
		return nil, errors.New("checkout session missing quotation_id metadata")
	}

	amount := FromStripeAmount(sess.AmountTotal)
	if amount <= 0 {
		if parsed, ok := metaChargeAmount(sess.Metadata); ok {
			amount = parsed
		}
	}
	if amount <= 0 {
		return nil, errors.New("checkout session has no payable amount")
	}

	var paymentIntentID string
	if sess.PaymentIntent != nil {
		paymentIntentID = strings.TrimSpace(sess.PaymentIntent.ID)
	}

	return &CheckoutPaymentDetails{
		QuotationID:     quotationID,
		Amount:          amount,
		SessionID:       sess.ID,
		PaymentIntentID: paymentIntentID,
	}, nil
}

// ParsePaymentIntentPayment extracts quotation payment from a succeeded PaymentIntent.
func ParsePaymentIntentPayment(pi *stripe.PaymentIntent) (*CheckoutPaymentDetails, error) {
	if pi == nil || strings.TrimSpace(pi.ID) == "" {
		return nil, errors.New("missing payment intent")
	}
	if pi.Status != stripe.PaymentIntentStatusSucceeded {
		return nil, fmt.Errorf("payment intent status is %q", pi.Status)
	}

	quotationID, err := parseUintMeta(pi.Metadata, "quotation_id")
	if err != nil || quotationID == 0 {
		return nil, errors.New("payment intent missing quotation_id metadata")
	}

	amount := FromStripeAmount(pi.Amount)
	if amount <= 0 {
		if parsed, ok := metaChargeAmount(pi.Metadata); ok {
			amount = parsed
		}
	}
	if amount <= 0 {
		return nil, errors.New("payment intent has no payable amount")
	}

	sessionID := strings.TrimSpace(pi.Metadata["checkout_session_id"])
	if sessionID == "" {
		sessionID = "pi:" + pi.ID
	}

	return &CheckoutPaymentDetails{
		QuotationID:     quotationID,
		Amount:          amount,
		SessionID:       sessionID,
		PaymentIntentID: pi.ID,
	}, nil
}

func metaChargeAmount(meta map[string]string) (float64, bool) {
	if meta == nil {
		return 0, false
	}
	raw := strings.TrimSpace(meta["charge_amount"])
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func parseUintMeta(meta map[string]string, key string) (uint, error) {
	if meta == nil {
		return 0, errors.New("missing metadata")
	}
	raw := strings.TrimSpace(meta[key])
	if raw == "" {
		return 0, fmt.Errorf("missing %s", key)
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(n), nil
}
