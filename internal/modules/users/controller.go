package users

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"my-app/internal/middleware"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func parseIDParam(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return 0, false
	}
	return uint(id64), true
}

func (ctrl *Controller) GetAll(c *gin.Context) {
	users, err := ctrl.service.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (ctrl *Controller) GetExaminers(c *gin.Context) {
	users, err := ctrl.service.GetExaminers(c.Query("search"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch examiners"})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (ctrl *Controller) GetByID(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	user, err := ctrl.service.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (ctrl *Controller) Create(c *gin.Context) {
	var input CreateUserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := ctrl.service.Create(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, user)
}

func (ctrl *Controller) UpdateStatus(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	var input UpdateStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := ctrl.service.UpdateStatus(id, input.Status)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (ctrl *Controller) UpdateRole(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	var input UpdateRoleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := ctrl.service.UpdateRole(id, input.RoleID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (ctrl *Controller) RequirePasswordReset(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	user, err := ctrl.service.RequirePasswordReset(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

func currentUserID(c *gin.Context) (uint, bool) {
	val, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return 0, false
	}
	id, ok := val.(uint)
	if !ok || id == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return 0, false
	}
	return id, true
}

func (ctrl *Controller) GetMe(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	user, err := ctrl.service.GetMe(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (ctrl *Controller) GetMyPermissions(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	states, err := ctrl.service.GetUserPermissions(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	names := make([]string, 0, len(states))
	for _, s := range states {
		if s.Effective {
			names = append(names, s.Name)
		}
	}
	c.JSON(http.StatusOK, gin.H{"permissions": names})
}

func (ctrl *Controller) UpdateMe(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var input UpdateMeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := ctrl.service.UpdateMe(userID, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (ctrl *Controller) DeleteMe(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := ctrl.service.DeleteMe(userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "account deleted"})
}

func (ctrl *Controller) Delete(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	actorID, ok := currentUserID(c)
	if !ok {
		return
	}
	if err := ctrl.service.DeleteUser(id, actorID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

func (ctrl *Controller) GetPermissions(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	perms, err := ctrl.service.GetUserPermissions(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, perms)
}

func (ctrl *Controller) SetPermissions(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var input SetPermissionsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.service.SetUserPermissions(id, input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Permission decisions are cached per user — clear so the change takes effect immediately.
	middleware.InvalidatePermissionCache()
	c.JSON(http.StatusOK, gin.H{"message": "permissions updated"})
}

func (ctrl *Controller) GetActivity(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "25"))
	logs, err := ctrl.service.GetActivity(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch activity logs"})
		return
	}
	c.JSON(http.StatusOK, logs)
}
