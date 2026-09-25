package agreements

import "github.com/gin-gonic/gin"

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	t := router.Group("/agreements/templates")
	{
		t.GET("", permissionMiddleware("agreement:view"), ctrl.ListTemplates)
		t.GET("/:id", permissionMiddleware("agreement:view"), ctrl.GetTemplate)
		t.POST("", permissionMiddleware("agreement:manage"), ctrl.CreateTemplate)
		t.PUT("/:id", permissionMiddleware("agreement:manage"), ctrl.UpdateTemplate)
		t.PATCH("/:id/active", permissionMiddleware("agreement:manage"), ctrl.SetTemplateActive)
		t.DELETE("/:id", permissionMiddleware("agreement:manage"), ctrl.DeleteTemplate)
	}

	router.GET("/agreements/booking-status", permissionMiddleware("agreement:view"), ctrl.BookingStatuses)

	r := router.Group("/agreements/requests")
	{
		r.GET("/:id", permissionMiddleware("agreement:view"), ctrl.GetRequest)
		r.GET("/:id/pdf", permissionMiddleware("agreement:view"), ctrl.StaffPDF)
		r.POST("/:id/resend", permissionMiddleware("agreement:send"), ctrl.ResendRequest)
		r.POST("/:id/void", permissionMiddleware("agreement:send"), ctrl.VoidRequest)
	}

	c := router.Group("/clients")
	{
		c.GET("/:id/agreements", permissionMiddleware("agreement:view"), ctrl.ListClientRequests)
		c.POST("/:id/agreements", permissionMiddleware("agreement:send"), ctrl.SendRequest)
	}
}

// RegisterPublicRoutes mounts the client signing endpoints. The caller passes the
// already-"/api/public" group, so this group must be just "/agreements".
func RegisterPublicRoutes(router *gin.RouterGroup, ctrl *Controller) {
	p := router.Group("/agreements")
	{
		p.GET("/:token", ctrl.GetPublicView)
		p.POST("/:token/sign", ctrl.SignPublic)
		p.POST("/:token/decline", ctrl.DeclinePublic)
		p.GET("/:token/pdf", ctrl.PublicPDF)
	}
}
