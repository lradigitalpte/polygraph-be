package exams

import (
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, ctrl *Controller, permissionMiddleware func(string) gin.HandlerFunc) {
	e := router.Group("/exams")
	{
		e.GET("", permissionMiddleware("exam:view"), ctrl.GetAllExams)
		e.POST("", permissionMiddleware("exam:create"), ctrl.CreateExam)
		e.GET("/appointment/:appointmentId", permissionMiddleware("exam:view"), ctrl.GetExamByAppointment)
		e.POST("/appointment/:appointmentId/start", permissionMiddleware("exam:conduct"), ctrl.StartDocumentation)
		e.GET("/types", permissionMiddleware("examtype:view"), ctrl.GetAllExamTypes)
		e.POST("/types", permissionMiddleware("examtype:create"), ctrl.CreateExamType)
		e.PATCH("/types/:id", permissionMiddleware("examtype:edit"), ctrl.UpdateExamType)
		e.DELETE("/types/:id", permissionMiddleware("examtype:delete"), ctrl.DeleteExamType)
		e.GET("/:id", permissionMiddleware("exam:view"), ctrl.GetExam)
		e.PATCH("/:id", permissionMiddleware("exam:conduct"), ctrl.UpdateExam)
		e.GET("/:id/intelligence", permissionMiddleware("exam:view"), ctrl.GetIntelligence)
		e.POST("/referral", permissionMiddleware("client:manage"), ctrl.CreateReferral)
		e.POST("/assessment", permissionMiddleware("exam:conduct"), ctrl.CreateAssessment)
		e.POST("/phase", permissionMiddleware("exam:conduct"), ctrl.AddPhase)
	}

	r := router.Group("/reports")
	{
		r.POST("", permissionMiddleware("exam:report"), ctrl.CreateReport)
		r.GET("/:id", permissionMiddleware("exam:view"), ctrl.GetReport)
		r.POST("/documents", permissionMiddleware("document:manage"), ctrl.UploadDocument)
		r.GET("/:id/documents", permissionMiddleware("exam:view"), ctrl.GetDocuments)
	}
}
