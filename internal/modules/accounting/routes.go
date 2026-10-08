package accounting

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	group := router.Group("/accounting")
	{
		group.GET("/expenses", permissionMiddleware("accounting:view"), ctrl.ListExpenses)
		group.GET("/expenses/:id", permissionMiddleware("accounting:view"), ctrl.GetExpense)
		group.POST("/expenses", permissionMiddleware("accounting:manage"), ctrl.CreateExpense)
		group.POST("/expenses/:id/receipt", permissionMiddleware("accounting:manage"), ctrl.UploadExpenseReceipt)
		group.GET("/expenses/:id/receipt", permissionMiddleware("accounting:view"), ctrl.GetExpenseReceipt)
		group.PATCH("/expenses/:id", permissionMiddleware("accounting:manage"), ctrl.UpdateExpense)
		group.DELETE("/expenses/:id", permissionMiddleware("accounting:manage"), ctrl.DeleteExpense)
		group.GET("/payment-methods", permissionMiddleware("accounting:view"), ctrl.ListPaymentMethods)
		group.POST("/payment-methods", permissionMiddleware("accounting:manage"), ctrl.CreatePaymentMethod)
		group.PATCH("/payment-methods/:id", permissionMiddleware("accounting:manage"), ctrl.UpdatePaymentMethod)
		group.DELETE("/payment-methods/:id", permissionMiddleware("accounting:manage"), ctrl.DeletePaymentMethod)
		group.GET("/purchase-items", permissionMiddleware("accounting:view"), ctrl.ListPurchaseItems)
		group.POST("/purchase-items", permissionMiddleware("accounting:manage"), ctrl.CreatePurchaseItem)
		group.PATCH("/purchase-items/:id", permissionMiddleware("accounting:manage"), ctrl.UpdatePurchaseItem)
		group.DELETE("/purchase-items/:id", permissionMiddleware("accounting:manage"), ctrl.DeletePurchaseItem)
		group.GET("/vat-return", permissionMiddleware("accounting:view"), ctrl.GetVatReturn)
		group.GET("/sales-report", permissionMiddleware("accounting:view"), ctrl.GetSalesReport)
	}
}
