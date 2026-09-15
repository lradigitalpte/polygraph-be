package settings

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	org := router.Group("/settings/organization")
	{
		org.GET("", permissionMiddleware("appointment:view"), ctrl.GetOrganization)
		org.PATCH("", permissionMiddleware("role:manage"), ctrl.UpdateOrganization)
		org.DELETE("", permissionMiddleware("role:manage"), ctrl.DeleteOrganization)
	}
}

// RegisterPublicRoutes mounts the unauthenticated organization-contact endpoint used
// by the public marketing/booking site. The caller passes the already-"/api/public"
// group, so this group must be just "/organization" (not "/public/organization").
func RegisterPublicRoutes(router *gin.RouterGroup, ctrl *Controller) {
	router.GET("/organization", ctrl.GetPublicOrganization)
}
