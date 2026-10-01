package payments

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
	"go.uber.org/zap"

	"my-app/internal/modules/appointments"
	"my-app/internal/stripeutil"
)

// Controller handles Stripe webhook callbacks.
type Controller struct {
	appointments *appointments.Service
	logger       *zap.Logger
}

// NewController wires Stripe webhook handling onto the appointments service.
func NewController(appService *appointments.Service, logger *zap.Logger) *Controller {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Controller{appointments: appService, logger: logger}
}

// HandleWebhook verifies the Stripe signature and applies successful Checkout payments.
func (ctrl *Controller) HandleWebhook(c *gin.Context) {
	const maxBodyBytes = int64(65536)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	secret := strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET"))
	if secret == "" {
		ctrl.logger.Error("STRIPE_WEBHOOK_SECRET is not configured")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "webhook not configured"})
		return
	}

	sig := c.GetHeader("Stripe-Signature")
	event, err := webhook.ConstructEvent(payload, sig, secret)
	if err != nil {
		ctrl.logger.Warn("stripe webhook signature verification failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
		return
	}

	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			ctrl.logger.Error("failed to parse checkout session", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event payload"})
			return
		}
		if err := ctrl.applyCheckoutSession(&sess); err != nil {
			ctrl.logger.Error("failed to apply stripe checkout payment",
				zap.String("session_id", sess.ID),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	default:
		// Acknowledge other events without action.
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}

func (ctrl *Controller) applyCheckoutSession(sess *stripe.CheckoutSession) error {
	if sess == nil || sess.ID == "" {
		return errors.New("missing checkout session")
	}
	if sess.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid &&
		sess.PaymentStatus != stripe.CheckoutSessionPaymentStatusNoPaymentRequired {
		ctrl.logger.Info("ignoring unpaid checkout session",
			zap.String("session_id", sess.ID),
			zap.String("payment_status", string(sess.PaymentStatus)),
		)
		return nil
	}

	quotationID, err := parseUintMeta(sess.Metadata, "quotation_id")
	if err != nil || quotationID == 0 {
		return fmt.Errorf("checkout session missing quotation_id metadata")
	}

	amount := stripeutil.FromStripeAmount(sess.AmountTotal)
	if amount <= 0 {
		if raw := strings.TrimSpace(sess.Metadata["charge_amount"]); raw != "" {
			if parsed, parseErr := strconv.ParseFloat(raw, 64); parseErr == nil {
				amount = parsed
			}
		}
	}
	if amount <= 0 {
		return errors.New("checkout session has no payable amount")
	}

	var paymentIntentID string
	if sess.PaymentIntent != nil {
		paymentIntentID = sess.PaymentIntent.ID
	}

	return ctrl.appointments.ApplyStripeCheckoutPayment(quotationID, amount, sess.ID, paymentIntentID)
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
