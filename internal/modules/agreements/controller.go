package agreements

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (ctrl *Controller) writeError(c *gin.Context, err error) {
	if errors.Is(err, ErrTemplateNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func (ctrl *Controller) ListTemplates(c *gin.Context) {
	rows, err := ctrl.service.ListTemplates(c.Query("include_inactive") == "true")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load agreements"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (ctrl *Controller) GetTemplate(c *gin.Context) {
	row, err := ctrl.service.GetTemplate(c.Param("id"))
	if err != nil {
		ctrl.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (ctrl *Controller) CreateTemplate(c *gin.Context) {
	var input TemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := ctrl.service.CreateTemplate(input, c.GetString("email"))
	if err != nil {
		ctrl.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (ctrl *Controller) UpdateTemplate(c *gin.Context) {
	var input TemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := ctrl.service.UpdateTemplate(c.Param("id"), input, c.GetString("email"))
	if err != nil {
		ctrl.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (ctrl *Controller) SetTemplateActive(c *gin.Context) {
	var input struct {
		Active *bool `json:"active" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "active is required"})
		return
	}
	row, err := ctrl.service.SetTemplateActive(c.Param("id"), *input.Active)
	if err != nil {
		ctrl.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (ctrl *Controller) DeleteTemplate(c *gin.Context) {
	if err := ctrl.service.DeleteTemplate(c.Param("id")); err != nil {
		ctrl.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Agreement deleted"})
}

// --- Agreement requests (staff) ---------------------------------------------------

func (ctrl *Controller) writeRequestError(c *gin.Context, err error) {
	if errors.Is(err, ErrRequestNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func (ctrl *Controller) ListClientRequests(c *gin.Context) {
	clientID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || clientID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}
	rows, err := ctrl.service.ListClientRequests(uint(clientID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load agreements"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (ctrl *Controller) SendRequest(c *gin.Context) {
	clientID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || clientID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}
	var input SendInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := ctrl.service.SendRequest(uint(clientID), input, c.GetString("email"))
	if err != nil {
		ctrl.writeRequestError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (ctrl *Controller) GetRequest(c *gin.Context) {
	row, err := ctrl.service.GetRequest(c.Param("id"))
	if err != nil {
		ctrl.writeRequestError(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (ctrl *Controller) ResendRequest(c *gin.Context) {
	row, err := ctrl.service.ResendRequest(c.Param("id"))
	if err != nil {
		ctrl.writeRequestError(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

func (ctrl *Controller) VoidRequest(c *gin.Context) {
	row, err := ctrl.service.VoidRequest(c.Param("id"), c.GetString("email"))
	if err != nil {
		ctrl.writeRequestError(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}

// --- Public signing page ------------------------------------------------------------

func (ctrl *Controller) writePublicError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrRequestNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "This agreement link is invalid. Please check the link in your email."})
	case errors.Is(err, ErrLinkUnavailable):
		c.JSON(http.StatusGone, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}

func (ctrl *Controller) GetPublicView(c *gin.Context) {
	view, err := ctrl.service.GetPublicView(c.Param("token"))
	if err != nil {
		ctrl.writePublicError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, view)
}

func (ctrl *Controller) SignPublic(c *gin.Context) {
	// A drawn signature is well under this; the cap stops oversized bodies on a public route.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var input SignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission"})
		return
	}
	view, err := ctrl.service.Sign(c.Param("token"), input, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		ctrl.writePublicError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (ctrl *Controller) DeclinePublic(c *gin.Context) {
	var input struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&input)
	view, err := ctrl.service.Decline(c.Param("token"), input.Reason)
	if err != nil {
		ctrl.writePublicError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func writePDF(c *gin.Context, data []byte, filename string) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/pdf", data)
}

func (ctrl *Controller) PublicPDF(c *gin.Context) {
	data, filename, err := ctrl.service.PublicPDF(c.Param("token"))
	if err != nil {
		if errors.Is(err, ErrNotSigned) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		ctrl.writePublicError(c, err)
		return
	}
	writePDF(c, data, filename)
}

func (ctrl *Controller) StaffPDF(c *gin.Context) {
	data, filename, err := ctrl.service.StaffPDF(c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrNotSigned) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		ctrl.writeRequestError(c, err)
		return
	}
	writePDF(c, data, filename)
}

func (ctrl *Controller) BookingStatuses(c *gin.Context) {
	ids, err := ParseAppointmentIDs(c.Query("appointment_ids"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	statuses, err := ctrl.service.BookingStatuses(ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load agreement statuses"})
		return
	}
	c.JSON(http.StatusOK, statuses)
}
