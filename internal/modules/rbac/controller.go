package rbac

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

// GetPermissions godoc
// @Summary Get all permissions
// @Tags rbac
// @Produce json
// @Success 200 {array} Permission
// @Router /api/rbac/permissions [get]
func (ctrl *Controller) GetPermissions(c *gin.Context) {
	permissions, err := ctrl.service.GetAllPermissions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch permissions"})
		return
	}
	c.JSON(http.StatusOK, permissions)
}

// GetRoles godoc
// @Summary Get all roles
// @Tags rbac
// @Produce json
// @Success 200 {array} Role
// @Router /api/rbac/roles [get]
func (ctrl *Controller) GetRoles(c *gin.Context) {
	roles, err := ctrl.service.GetAllRoles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch roles"})
		return
	}
	c.JSON(http.StatusOK, roles)
}

// CreateRole godoc
// @Summary Create a new role
// @Tags rbac
// @Accept json
// @Produce json
// @Param role body map[string]interface{} true "Role Data"
// @Success 201 {object} Role
// @Router /api/rbac/roles [post]
func (ctrl *Controller) CreateRole(c *gin.Context) {
	var input struct {
		Name          string `json:"name" binding:"required"`
		Description   string `json:"description"`
		PermissionIDs []uint `json:"permission_ids"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role, err := ctrl.service.CreateRole(input.Name, input.Description, input.PermissionIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create role"})
		return
	}

	c.JSON(http.StatusCreated, role)
}
