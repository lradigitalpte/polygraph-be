package forms

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	f := router.Group("/forms")
	{
		f.GET("/templates", permissionMiddleware("client:manage"), ctrl.ListTemplates)
		f.GET("/requests/pending", permissionMiddleware("appointment:view"), ctrl.ListPendingRequests)
		f.POST("/requests/:id/resend", permissionMiddleware("client:manage"), ctrl.ResendRequest)
	}

	c := router.Group("/clients")
	{
		c.GET("/:id/form-requests", permissionMiddleware("client:manage"), ctrl.ListClientRequests)
		c.POST("/:id/form-requests", permissionMiddleware("client:manage"), ctrl.SendClientForm)
	}

	s := router.Group("/subjects")
	{
		s.GET("/:id/form-requests", permissionMiddleware("subject:view"), ctrl.ListSubjectRequests)
		s.POST("/:id/form-requests", permissionMiddleware("client:manage"), ctrl.SendSubjectForm)
	}
}

func RegisterPublicRoutes(router *gin.RouterGroup, ctrl *Controller) {
	p := router.Group("/public/forms")
	{
		p.GET("/:token", ctrl.GetPublicForm)
		p.POST("/:token", ctrl.SubmitPublicForm)
	}
}
