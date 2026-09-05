package exams

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (ctrl *Controller) ListAppointmentQuestions(c *gin.Context) {
	appointmentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment id"})
		return
	}
	questions, err := ctrl.service.ListAppointmentQuestions(uint(appointmentID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load prepared questions"})
		return
	}
	c.JSON(http.StatusOK, questions)
}

func (ctrl *Controller) ReplaceAppointmentQuestions(c *gin.Context) {
	appointmentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment id"})
		return
	}
	var body struct {
		Questions []AppointmentQuestionInput `json:"questions"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	questions, err := ctrl.service.ReplaceAppointmentQuestions(uint(appointmentID), body.Questions)
	if err != nil {
		writeAppointmentQuestionError(c, err)
		return
	}
	c.JSON(http.StatusOK, questions)
}

func (ctrl *Controller) AddAppointmentQuestionsFromTemplates(c *gin.Context) {
	appointmentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment id"})
		return
	}
	var body struct {
		TemplateIDs []uint `json:"template_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	questions, err := ctrl.service.AddAppointmentQuestionsFromTemplates(uint(appointmentID), body.TemplateIDs)
	if err != nil {
		writeAppointmentQuestionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, questions)
}

// writeAppointmentQuestionError maps the "documentation already started" case to
// 409 so the UI can switch to the session's own question endpoints.
func writeAppointmentQuestionError(c *gin.Context, err error) {
	if errors.Is(err, errDocumentationStarted) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
