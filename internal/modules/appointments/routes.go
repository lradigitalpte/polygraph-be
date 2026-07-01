package appointments

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	c := router.Group("/clients")
	{
		// Read endpoints — examiners get client:view (no billing). Account/billing stays client:manage.
		c.GET("", permissionMiddleware("client:view"), ctrl.GetClients)
		c.GET("/:id", permissionMiddleware("client:view"), ctrl.GetClient)
		c.GET("/:id/examinees", permissionMiddleware("client:view"), ctrl.GetClientExaminees)
		c.GET("/:id/appointments", permissionMiddleware("appointment:view"), ctrl.GetClientAppointments)
		c.GET("/:id/documents", permissionMiddleware("client:view"), ctrl.GetClientDocuments)
		// Billing — restricted to client:manage (examiners excluded).
		c.GET("/:id/account", permissionMiddleware("client:manage"), ctrl.GetClientAccount)
		// Write endpoints — client:manage.
		c.POST("", permissionMiddleware("client:manage"), ctrl.CreateClient)
		c.PATCH("/:id", permissionMiddleware("client:manage"), ctrl.UpdateClient)
		c.DELETE("/:id", permissionMiddleware("client:manage"), ctrl.DeleteClient)
		c.POST("/:id/examinees", permissionMiddleware("subject:create"), ctrl.BulkCreateExaminees)
		c.POST("/:id/documents", permissionMiddleware("document:manage"), ctrl.UploadClientDocument)
		c.POST("/:id/documents/form", permissionMiddleware("client:manage"), ctrl.CreateClientFormDocument)
	}

	a := router.Group("/appointments")
	{
		a.GET("", permissionMiddleware("appointment:view"), ctrl.GetAppointments)
		a.POST("", permissionMiddleware("appointment:create"), ctrl.CreateAppointment)
		a.POST("/bulk-schedule", permissionMiddleware("appointment:create"), ctrl.BulkSchedule)
		a.POST("/bulk-import-historical", permissionMiddleware("appointment:create"), ctrl.BulkImportHistorical)
		a.GET("/:id", permissionMiddleware("appointment:view"), ctrl.GetAppointment)
		a.PATCH("/:id", permissionMiddleware("appointment:manage"), ctrl.UpdateAppointment)
		a.DELETE("/:id", permissionMiddleware("appointment:manage"), ctrl.DeleteAppointment)
		a.PATCH("/:id/status", permissionMiddleware("appointment:manage"), ctrl.UpdateStatus)
		a.PATCH("/:id/payment", permissionMiddleware("appointment:manage"), ctrl.UpdatePayment)
		a.PATCH("/:id/collect-payment", permissionMiddleware("appointment:manage"), ctrl.CollectAppointmentPayment)
		a.PATCH("/:id/send-payment-reminder", permissionMiddleware("appointment:manage"), ctrl.SendAppointmentPaymentReminder)
		a.GET("/billing/ledger", permissionMiddleware("appointment:view"), ctrl.GetBillingLedger)
	}

	sub := router.Group("/subjects")
	{
		sub.GET("/:id/appointments", permissionMiddleware("appointment:view"), ctrl.GetSubjectAppointments)
		sub.GET("/:id/documents", permissionMiddleware("subject:view"), ctrl.GetSubjectDocuments)
		sub.POST("/:id/documents", permissionMiddleware("document:manage"), ctrl.UploadSubjectDocument)
	}

	b := router.Group("/billing")
	{
		b.GET("/ledger", permissionMiddleware("appointment:view"), ctrl.GetBillingLedger)
		b.POST("/bulk-edit-prices", permissionMiddleware("payment:manage"), ctrl.BulkEditPrices)
	}

	q := router.Group("/quotations")
	{
		q.GET("", permissionMiddleware("appointment:view"), ctrl.GetQuotations)
		q.POST("", permissionMiddleware("appointment:manage"), ctrl.CreateQuotation)
		q.PATCH("/:id/send-email", permissionMiddleware("appointment:manage"), ctrl.SendQuotationEmail)
		q.PATCH("/:id/collect-payment", permissionMiddleware("appointment:manage"), ctrl.CollectQuotationPayment)
		q.POST("/:id/convert", permissionMiddleware("appointment:create"), ctrl.ConvertQuotation)
		q.DELETE("/:id", permissionMiddleware("payment:manage"), ctrl.DeleteQuotation)
	}

	ds := router.Group("/document-shares")
	{
		ds.POST("", permissionMiddleware("document:manage"), ctrl.CreateDocumentShare)
		ds.GET("", permissionMiddleware("client:view"), ctrl.ListDocumentShares)
		ds.POST("/:id/resend", permissionMiddleware("document:manage"), ctrl.ResendDocumentShare)
	}
}

// RegisterPublicRoutes mounts the unauthenticated document-view endpoint. The
// caller passes the already-"/api/public" group, so this group must be just
// "/shared-docs" (not "/public/shared-docs") to avoid a doubled path prefix.
func RegisterPublicRoutes(router *gin.RouterGroup, ctrl *Controller) {
	p := router.Group("/shared-docs")
	{
		p.GET("/:token", ctrl.GetPublicDocumentShare)
	}
}
