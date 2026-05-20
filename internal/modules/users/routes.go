package users

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	me := router.Group("/me")
	{
		me.GET("", ctrl.GetMe)
		me.PATCH("", ctrl.UpdateMe)
		me.DELETE("", ctrl.DeleteMe)
	}

	users := router.Group("/users")
	{
		users.GET("", permissionMiddleware("user:view"), ctrl.GetAll)
		users.GET("/examiners", permissionMiddleware("user:view"), ctrl.GetExaminers)
		users.POST("", permissionMiddleware("user:create"), ctrl.Create)
		users.GET("/:id", permissionMiddleware("user:view"), ctrl.GetByID)
		users.GET("/:id/activity", permissionMiddleware("user:view"), ctrl.GetActivity)
		users.PATCH("/:id/status", permissionMiddleware("user:edit"), ctrl.UpdateStatus)
		users.PATCH("/:id/role", permissionMiddleware("user:edit"), ctrl.UpdateRole)
		users.POST("/:id/require-password-reset", permissionMiddleware("user:edit"), ctrl.RequirePasswordReset)
		users.DELETE("/:id", permissionMiddleware("user:delete"), ctrl.Delete)
	}
}
