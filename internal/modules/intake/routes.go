package intake

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	r := router.Group("/intake-requests")
	{
		r.POST("", permissionMiddleware("client:manage"), ctrl.SendIntakeRequest)
		r.GET("", permissionMiddleware("client:manage"), ctrl.ListIntakeRequests)
		r.POST("/:id/resend", permissionMiddleware("client:manage"), ctrl.ResendIntakeRequest)
		r.GET("/:id/submission", permissionMiddleware("client:manage"), ctrl.GetSubmission)
	}
}

func RegisterPublicRoutes(router *gin.RouterGroup, ctrl *Controller) {
	p := router.Group("/intake")
	{
		p.GET("/:token", ctrl.GetPublicIntakeForm)
		p.POST("/:token", ctrl.SubmitIntakeForm)
	}
}
