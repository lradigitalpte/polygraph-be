package subjects

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

// Create godoc
// @Summary Register a new examinee
// @Tags forensic
// @Accept json
// @Produce json
// @Param subject body Subject true "Subject Data"
// @Success 201 {object} Subject
// @Router /api/subjects [post]
func (ctrl *Controller) Create(c *gin.Context) {
	var subject Subject
	if err := c.ShouldBindJSON(&subject); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.service.Create(&subject); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create subject"})
		return
	}

	c.JSON(http.StatusCreated, subject)
}

// GetAll godoc
// @Summary Get all examinees
// @Tags forensic
// @Produce json
// @Success 200 {array} Subject
// @Router /api/subjects [get]
func (ctrl *Controller) GetAll(c *gin.Context) {
	subjects, err := ctrl.service.GetAll(c.Query("search"), c.Query("client_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch subjects"})
		return
	}
	c.JSON(http.StatusOK, subjects)
}

func (ctrl *Controller) GetByID(c *gin.Context) {
	subject, err := ctrl.service.GetByID(c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "subject not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, subject)
}

func (ctrl *Controller) Update(c *gin.Context) {
	var input Subject
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.service.Update(c.Param("id"), &input); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "subject not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	subject, err := ctrl.service.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load updated subject"})
		return
	}
	c.JSON(http.StatusOK, subject)
}
