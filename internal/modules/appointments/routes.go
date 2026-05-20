package appointments

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	c := router.Group("/clients")
	{
		c.GET("", permissionMiddleware("client:manage"), ctrl.GetClients)
		c.POST("", permissionMiddleware("client:manage"), ctrl.CreateClient)
		c.GET("/:id", permissionMiddleware("client:manage"), ctrl.GetClient)
		c.GET("/:id/account", permissionMiddleware("client:manage"), ctrl.GetClientAccount)
		c.PATCH("/:id", permissionMiddleware("client:manage"), ctrl.UpdateClient)
		c.DELETE("/:id", permissionMiddleware("client:manage"), ctrl.DeleteClient)
		c.GET("/:id/examinees", permissionMiddleware("client:manage"), ctrl.GetClientExaminees)
		c.POST("/:id/examinees", permissionMiddleware("subject:create"), ctrl.BulkCreateExaminees)
		c.GET("/:id/appointments", permissionMiddleware("appointment:view"), ctrl.GetClientAppointments)
		c.GET("/:id/documents", permissionMiddleware("client:manage"), ctrl.GetClientDocuments)
		c.POST("/:id/documents", permissionMiddleware("document:manage"), ctrl.UploadClientDocument)
		c.POST("/:id/documents/form", permissionMiddleware("client:manage"), ctrl.CreateClientFormDocument)
	}

	a := router.Group("/appointments")
	{
		a.GET("", permissionMiddleware("appointment:view"), ctrl.GetAppointments)
		a.POST("", permissionMiddleware("appointment:create"), ctrl.CreateAppointment)
		a.GET("/:id", permissionMiddleware("appointment:view"), ctrl.GetAppointment)
		a.PATCH("/:id", permissionMiddleware("appointment:manage"), ctrl.UpdateAppointment)
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
	}

	q := router.Group("/quotations")
	{
		q.GET("", permissionMiddleware("appointment:view"), ctrl.GetQuotations)
		q.POST("", permissionMiddleware("appointment:manage"), ctrl.CreateQuotation)
		q.PATCH("/:id/send-email", permissionMiddleware("appointment:manage"), ctrl.SendQuotationEmail)
		q.PATCH("/:id/collect-payment", permissionMiddleware("appointment:manage"), ctrl.CollectQuotationPayment)
	}
}
