package forms

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (ctrl *Controller) ListPendingRequests(c *gin.Context) {
	requests, err := ctrl.service.ListPendingRequests(50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load pending forms"})
		return
	}
	c.JSON(http.StatusOK, requests)
}

func (ctrl *Controller) ListTemplates(c *gin.Context) {
	templates, err := ctrl.service.ListTemplates(c.Query("audience"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load templates"})
		return
	}
	c.JSON(http.StatusOK, templates)
}

func (ctrl *Controller) ListClientRequests(c *gin.Context) {
	requests, err := ctrl.service.ListClientRequests(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load form requests"})
		return
	}
	c.JSON(http.StatusOK, requests)
}

func (ctrl *Controller) SendClientForm(c *gin.Context) {
	clientID, err := ParseClientIDParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var input struct {
		TemplateID     uint   `json:"template_id" binding:"required"`
		SubjectID      *uint  `json:"subject_id"`
		RecipientEmail string `json:"recipient_email"`
		RecipientName  string `json:"recipient_name"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req, err := ctrl.service.SendFormRequest(SendFormInput{
		TemplateID:     input.TemplateID,
		ClientID:       clientID,
		SubjectID:      input.SubjectID,
		RecipientEmail: input.RecipientEmail,
		RecipientName:  input.RecipientName,
		SentByEmail:    c.GetString("user_email"),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, req)
}

func (ctrl *Controller) ListSubjectRequests(c *gin.Context) {
	requests, err := ctrl.service.ListSubjectRequests(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load form requests"})
		return
	}
	c.JSON(http.StatusOK, requests)
}

func (ctrl *Controller) SendSubjectForm(c *gin.Context) {
	subjectID, err := ParseClientIDParam(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var input struct {
		TemplateID     uint   `json:"template_id" binding:"required"`
		ClientID       uint   `json:"client_id" binding:"required"`
		RecipientEmail string `json:"recipient_email"`
		RecipientName  string `json:"recipient_name"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req, err := ctrl.service.SendFormRequest(SendFormInput{
		TemplateID:     input.TemplateID,
		ClientID:       input.ClientID,
		SubjectID:      &subjectID,
		RecipientEmail: input.RecipientEmail,
		RecipientName:  input.RecipientName,
		SentByEmail:    c.GetString("user_email"),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, req)
}

func (ctrl *Controller) ResendRequest(c *gin.Context) {
	req, err := ctrl.service.ResendFormRequest(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, req)
}

func (ctrl *Controller) GetPublicForm(c *gin.Context) {
	view, err := ctrl.service.GetPublicForm(c.Param("token"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

func (ctrl *Controller) SubmitPublicForm(c *gin.Context) {
	var input struct {
		FormData map[string]interface{} `json:"form_data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	req, err := ctrl.service.SubmitPublicForm(c.Param("token"), input.FormData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Form submitted successfully",
		"request": req,
	})
}
