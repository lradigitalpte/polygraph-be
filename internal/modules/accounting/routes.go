package accounting

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	group := router.Group("/accounting")
	{
		group.GET("/expenses", permissionMiddleware("accounting:view"), ctrl.ListExpenses)
		group.GET("/expenses/:id", permissionMiddleware("accounting:view"), ctrl.GetExpense)
		group.POST("/expenses", permissionMiddleware("accounting:manage"), ctrl.CreateExpense)
		group.PATCH("/expenses/:id", permissionMiddleware("accounting:manage"), ctrl.UpdateExpense)
		group.DELETE("/expenses/:id", permissionMiddleware("accounting:manage"), ctrl.DeleteExpense)
		group.GET("/vat-return", permissionMiddleware("accounting:view"), ctrl.GetVatReturn)
		group.GET("/sales-report", permissionMiddleware("accounting:view"), ctrl.GetSalesReport)
	}
}
