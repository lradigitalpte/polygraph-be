package settings

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	org := router.Group("/settings/organization")
	{
		org.GET("", permissionMiddleware("role:manage"), ctrl.GetOrganization)
		org.PATCH("", permissionMiddleware("role:manage"), ctrl.UpdateOrganization)
		org.DELETE("", permissionMiddleware("role:manage"), ctrl.DeleteOrganization)
	}
}
