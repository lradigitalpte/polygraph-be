package subjects

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	s := router.Group("/subjects")
	{
		s.GET("", permissionMiddleware("subject:view"), ctrl.GetAll)
		s.POST("", permissionMiddleware("subject:create"), ctrl.Create)
		s.GET("/:id", permissionMiddleware("subject:view"), ctrl.GetByID)
		s.PATCH("/:id", permissionMiddleware("subject:edit"), ctrl.Update)
	}
}
