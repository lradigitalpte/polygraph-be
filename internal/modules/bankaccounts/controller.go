package bankaccounts

import (
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

func (ctrl *Controller) List(c *gin.Context) {
	rows, err := ctrl.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load bank accounts"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (ctrl *Controller) Create(c *gin.Context) {
	var input Input
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := ctrl.service.Create(input, actorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (ctrl *Controller) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input Input
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := ctrl.service.Update(id, input, actorID(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

func (ctrl *Controller) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := ctrl.service.Delete(id, actorID(c)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return uint(id), true
}

func actorID(c *gin.Context) *uint {
	if val, exists := c.Get("user_id"); exists {
		if id, ok := val.(uint); ok {
			return &id
		}
	}
	return nil
}
