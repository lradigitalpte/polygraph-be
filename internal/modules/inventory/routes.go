package inventory

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	group := router.Group("/inventory")
	{
		group.GET("", permissionMiddleware("user:view"), ctrl.ListItems)
		group.GET("/:id", permissionMiddleware("user:view"), ctrl.GetItem)
		group.POST("", permissionMiddleware("user:view"), ctrl.CreateItem)
		group.PATCH("/:id", permissionMiddleware("user:view"), ctrl.UpdateItem)
		group.DELETE("/:id", permissionMiddleware("user:view"), ctrl.DeleteItem)
	}
}
