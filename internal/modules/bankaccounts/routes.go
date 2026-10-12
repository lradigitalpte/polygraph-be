package bankaccounts

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	g := router.Group("/settings/bank-accounts")
	{
		// Staff creating invoices need the list to pick from; only admins may change it.
		g.GET("", permissionMiddleware("appointment:view"), ctrl.List)
		g.POST("", permissionMiddleware("role:manage"), ctrl.Create)
		g.PATCH("/:id", permissionMiddleware("role:manage"), ctrl.Update)
		g.DELETE("/:id", permissionMiddleware("role:manage"), ctrl.Delete)
	}
}
