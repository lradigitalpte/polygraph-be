package auditlogs

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the audit log endpoints on the given router group.
func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	audit := router.Group("/audit-logs")
	{
		audit.GET("", permissionMiddleware("audit:view"), ctrl.GetAll)
		audit.GET("/:id", permissionMiddleware("audit:view"), ctrl.GetByID)
	}
}
