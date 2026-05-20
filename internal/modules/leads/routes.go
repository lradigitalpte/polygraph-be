package leads

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the leads endpoints on the given router group.
func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	leads := router.Group("/leads")
	{
		leads.GET("", permissionMiddleware("lead:view"), ctrl.GetAll)
		leads.POST("", permissionMiddleware("lead:create"), ctrl.Create)
		leads.GET("/:id", permissionMiddleware("lead:view"), ctrl.GetByID)
		leads.PATCH("/:id", permissionMiddleware("lead:update"), ctrl.Update)
		leads.DELETE("/:id", permissionMiddleware("lead:delete"), ctrl.Delete)
	}
}
