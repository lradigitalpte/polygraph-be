package payments

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
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
	event, err := webhook.ConstructEventWithOptions(payload, sig, secret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
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
		if err := ctrl.applyCheckoutSessionID(sess.ID, &sess); err != nil {
			ctrl.logger.Error("failed to apply stripe checkout payment",
				zap.String("session_id", sess.ID),
				zap.String("event_type", string(event.Type)),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	case "payment_intent.succeeded":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			ctrl.logger.Error("failed to parse payment intent", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event payload"})
			return
		}
		details, err := stripeutil.ParsePaymentIntentPayment(&pi)
		if err != nil {
			ctrl.logger.Info("ignoring payment_intent.succeeded",
				zap.String("payment_intent_id", pi.ID),
				zap.Error(err),
			)
			break
		}
		if err := ctrl.appointments.ApplyStripeCheckoutPayment(
			details.QuotationID,
			details.Amount,
			details.SessionID,
			details.PaymentIntentID,
		); err != nil {
			ctrl.logger.Error("failed to apply stripe payment intent",
				zap.String("payment_intent_id", pi.ID),
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

func (ctrl *Controller) applyCheckoutSessionID(sessionID string, fallback *stripe.CheckoutSession) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}

	sess, err := stripeutil.RetrieveCheckoutSession(sessionID)
	if err != nil {
		ctrl.logger.Warn("stripe session retrieve failed, using webhook payload",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		sess = fallback
	}

	details, err := stripeutil.ParseCheckoutSessionPayment(sess)
	if err != nil {
		if fallback != nil && stripeutil.CheckoutSessionIsPaid(fallback) {
			details, err = stripeutil.ParseCheckoutSessionPayment(fallback)
		}
	}
	if err != nil {
		ctrl.logger.Info("checkout session not applied",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return nil
	}

	return ctrl.appointments.ApplyStripeCheckoutPayment(
		details.QuotationID,
		details.Amount,
		details.SessionID,
		details.PaymentIntentID,
	)
}
