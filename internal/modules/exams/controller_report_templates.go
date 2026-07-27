package exams

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (ctrl *Controller) ListReportTemplates(c *gin.Context) {
	includeInactive := c.Query("include_inactive") == "true"
	templates, err := ctrl.service.ListReportTemplates(includeInactive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load report templates"})
		return
	}
	c.JSON(http.StatusOK, templates)
}

func (ctrl *Controller) GetReportTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template id"})
		return
	}
	template, err := ctrl.service.GetReportTemplateByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}
	c.JSON(http.StatusOK, template)
}

func (ctrl *Controller) ResolveReportTemplate(c *gin.Context) {
	clientID, _ := strconv.ParseUint(c.Query("client_id"), 10, 64)
	template, err := ctrl.service.ResolveReportTemplate(uint(clientID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No report template found"})
		return
	}
	c.JSON(http.StatusOK, template)
}

func (ctrl *Controller) CreateReportTemplate(c *gin.Context) {
	var input ReportTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	template, err := ctrl.service.CreateReportTemplate(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, template)
}

func (ctrl *Controller) UpdateReportTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template id"})
		return
	}
	var input ReportTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	template, err := ctrl.service.UpdateReportTemplate(uint(id), input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, template)
}

func (ctrl *Controller) DeleteReportTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template id"})
		return
	}
	if err := ctrl.service.DeactivateReportTemplate(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
