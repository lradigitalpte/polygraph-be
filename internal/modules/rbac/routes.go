package rbac

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller) {
	rbac := router.Group("/rbac")
	{
		rbac.GET("/permissions", ctrl.GetPermissions)
		rbac.GET("/roles", ctrl.GetRoles)
		rbac.POST("/roles", ctrl.CreateRole)
		rbac.PUT("/roles/:id", ctrl.UpdateRole)
	}
}
