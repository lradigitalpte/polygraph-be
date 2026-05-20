package auditlogs

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Controller exposes HTTP handlers for audit logs.
type Controller struct {
	service *Service
}

// NewController creates a Controller backed by the given Service.
func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

// GetAll godoc
// @Summary      List audit logs
// @Tags         audit-logs
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query     int  false  "Maximum rows (default 100, max 500)"
// @Success      200    {array}   AuditLogRow
// @Failure      400    {object}  map[string]string
// @Failure      401    {object}  map[string]string
// @Failure      403    {object}  map[string]string
// @Failure      500    {object}  map[string]string
// @Router       /audit-logs [get]
func (ctrl *Controller) GetAll(c *gin.Context) {
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit value"})
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = parsed
	}

	rows, err := ctrl.service.GetAll(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch audit logs"})
		return
	}

	c.JSON(http.StatusOK, rows)
}

// GetByID godoc
// @Summary      Get an audit log entry
// @Tags         audit-logs
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Audit log ID"
// @Success      200  {object}  AuditLogRow
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /audit-logs/{id} [get]
func (ctrl *Controller) GetByID(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid audit log ID"})
		return
	}

	row, err := ctrl.service.GetByID(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Audit log not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch audit log"})
		return
	}

	c.JSON(http.StatusOK, row)
}
