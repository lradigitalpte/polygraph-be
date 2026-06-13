package intake

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
}

func NewController(svc *Service) *Controller {
	return &Controller{service: svc}
}

// ─── Admin endpoints (authenticated) ─────────────────────────────────────────

// POST /api/intake-requests
func (ctrl *Controller) SendIntakeRequest(c *gin.Context) {
	var body struct {
		ClientID       uint   `json:"client_id"       binding:"required"`
		RecipientEmail string `json:"recipient_email" binding:"required"`
		RecipientName  string `json:"recipient_name"`
		Message        string `json:"message"`
		ExpiryDays     int    `json:"expiry_days"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sentBy := ""
	if email, ok := c.Get("user_email"); ok {
		sentBy, _ = email.(string)
	}

	req, err := ctrl.service.SendIntakeRequest(
		body.ClientID,
		body.RecipientEmail,
		body.RecipientName,
		body.Message,
		sentBy,
		body.ExpiryDays,
	)
	if err != nil {
		// If the record was created but email failed, still return 201 with the error.
		if req != nil {
			c.JSON(http.StatusCreated, gin.H{
				"intake_request": req,
				"warning":        err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, req)
}

// GET /api/intake-requests?client_id=
func (ctrl *Controller) ListIntakeRequests(c *gin.Context) {
	list, err := ctrl.service.ListIntakeRequests(c.Query("client_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load intake requests"})
		return
	}
	c.JSON(http.StatusOK, list)
}

// POST /api/intake-requests/:id/resend
func (ctrl *Controller) ResendIntakeRequest(c *gin.Context) {
	req, err := ctrl.service.ResendIntakeRequest(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, req)
}

// GET /api/intake-requests/:id/submission
func (ctrl *Controller) GetSubmission(c *gin.Context) {
	sub, err := ctrl.service.GetSubmission(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sub)
}

// ─── Public endpoints (no auth) ──────────────────────────────────────────────

// GET /api/public/intake/:token
func (ctrl *Controller) GetPublicIntakeForm(c *gin.Context) {
	req, err := ctrl.service.GetPublicRequest(c.Param("token"))
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "intake form not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	// Return only what the public page needs — no internal IDs.
	c.JSON(http.StatusOK, gin.H{
		"client_name":    req.ClientName,
		"recipient_name": req.RecipientName,
		"message":        req.Message,
		"expires_at":     req.ExpiresAt,
		"status":         req.Status,
	})
}

// POST /api/public/intake/:token
func (ctrl *Controller) SubmitIntakeForm(c *gin.Context) {
	var body struct {
		Subjects []SubjectInput `json:"subjects" binding:"required,min=1"`
		Agreed   bool           `json:"agreed"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := ctrl.service.SubmitIntakeRequest(c.Param("token"), body.Subjects, body.Agreed)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "intake form not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Thank you. Your submission has been received.",
		"created": len(created),
	})
}
