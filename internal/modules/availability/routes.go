package availability

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	a := router.Group("/availability")
	{
		a.GET("/blocks", permissionMiddleware("availability:view"), ctrl.ListBlocks)
		a.POST("/blocks", permissionMiddleware("availability:manage"), ctrl.CreateBlock)
		a.PATCH("/blocks/:id", permissionMiddleware("availability:manage"), ctrl.UpdateBlock)
		a.DELETE("/blocks/:id", permissionMiddleware("availability:manage"), ctrl.DeleteBlock)
		a.GET("/examiners/:id", permissionMiddleware("availability:check"), ctrl.GetExaminerDayAvailability)
	}
}
