package settings

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (ctrl *Controller) GetOrganization(c *gin.Context) {
	row, err := ctrl.service.GetOrganization()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load organization settings"})
		return
	}
	c.JSON(http.StatusOK, row)
}

func (ctrl *Controller) UpdateOrganization(c *gin.Context) {
	var input UpdateOrganizationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	row, err := ctrl.service.UpdateOrganization(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

func (ctrl *Controller) DeleteOrganization(c *gin.Context) {
	var body struct {
		ConfirmName string `json:"confirm_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	row, err := ctrl.service.GetOrganization()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load organization settings"})
		return
	}
	if strings.TrimSpace(body.ConfirmName) != strings.TrimSpace(row.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation name does not match organization name"})
		return
	}

	if err := ctrl.service.DeleteOrganizationData(); err != nil {
		log.Printf("delete organization data failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete organization data"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Organization data deleted. Users and roles were kept."})
}
