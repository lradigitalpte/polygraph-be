package exams

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (ctrl *Controller) ListQuestionTemplates(c *gin.Context) {
	includeInactive := c.Query("include_inactive") == "true"
	var examTypeID *uint
	if raw := c.Query("exam_type_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exam_type_id"})
			return
		}
		v := uint(id)
		examTypeID = &v
	}
	templates, err := ctrl.service.ListQuestionTemplates(examTypeID, includeInactive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load question templates"})
		return
	}
	c.JSON(http.StatusOK, templates)
}

func (ctrl *Controller) GetQuestionTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template id"})
		return
	}
	template, err := ctrl.service.GetQuestionTemplateByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}
	c.JSON(http.StatusOK, template)
}

func (ctrl *Controller) CreateQuestionTemplate(c *gin.Context) {
	var input QuestionTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	template, err := ctrl.service.CreateQuestionTemplate(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, template)
}

func (ctrl *Controller) UpdateQuestionTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template id"})
		return
	}
	var input QuestionTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	template, err := ctrl.service.UpdateQuestionTemplate(uint(id), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, template)
}

func (ctrl *Controller) DeleteQuestionTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template id"})
		return
	}
	if err := ctrl.service.DeactivateQuestionTemplate(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
