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
		e.GET("/question-templates", permissionMiddleware("question_template:view"), ctrl.ListQuestionTemplates)
		e.GET("/question-templates/:id", permissionMiddleware("question_template:view"), ctrl.GetQuestionTemplate)
		e.POST("/question-templates", permissionMiddleware("question_template:manage"), ctrl.CreateQuestionTemplate)
		e.PATCH("/question-templates/:id", permissionMiddleware("question_template:manage"), ctrl.UpdateQuestionTemplate)
		e.DELETE("/question-templates/:id", permissionMiddleware("question_template:manage"), ctrl.DeleteQuestionTemplate)
		e.GET("/:id", permissionMiddleware("exam:view"), ctrl.GetExam)
		e.PATCH("/:id", permissionMiddleware("exam:conduct"), ctrl.UpdateExam)
		e.GET("/:id/intelligence", permissionMiddleware("exam:view"), ctrl.GetIntelligence)
		e.GET("/:id/questions", permissionMiddleware("exam:view"), ctrl.ListExamQuestions)
		e.POST("/:id/questions", permissionMiddleware("exam:conduct"), ctrl.CreateExamQuestion)
		e.PATCH("/:id/questions/:qid", permissionMiddleware("exam:conduct"), ctrl.UpdateExamQuestion)
		e.DELETE("/:id/questions/:qid", permissionMiddleware("exam:conduct"), ctrl.DeleteExamQuestion)
		e.POST("/:id/questions/from-templates", permissionMiddleware("exam:conduct"), ctrl.AddExamQuestionsFromTemplates)
		e.POST("/referral", permissionMiddleware("client:manage"), ctrl.CreateReferral)
		e.POST("/assessment", permissionMiddleware("exam:conduct"), ctrl.CreateAssessment)
		e.POST("/phase", permissionMiddleware("exam:conduct"), ctrl.AddPhase)
	}

	// Questions prepared for a booking before its examination record exists.
	// Mounted here (not in the appointments module) because the question
	// machinery lives in this package; appointments already imports exams, so the
	// dependency must not run the other way. The path param is ":id" to match the
	// appointments module's own /appointments/:id routes.
	//
	// Writes use appointment:create rather than appointment:manage: examiners hold
	// appointment:create by default but not appointment:manage, and preparing
	// questions is part of setting up a booking.
	aq := router.Group("/appointments")
	{
		aq.GET("/:id/questions", permissionMiddleware("appointment:view"), ctrl.ListAppointmentQuestions)
		aq.PUT("/:id/questions", permissionMiddleware("appointment:create"), ctrl.ReplaceAppointmentQuestions)
		aq.POST("/:id/questions/from-templates", permissionMiddleware("appointment:create"), ctrl.AddAppointmentQuestionsFromTemplates)
	}

	r := router.Group("/reports")
	{
		r.POST("", permissionMiddleware("exam:report"), ctrl.CreateReport)
		r.GET("/templates", permissionMiddleware("exam:report"), ctrl.ListReportTemplates)
		r.GET("/templates/resolve", permissionMiddleware("exam:report"), ctrl.ResolveReportTemplate)
		r.GET("/templates/:id", permissionMiddleware("exam:report"), ctrl.GetReportTemplate)
		r.POST("/templates", permissionMiddleware("report_template:manage"), ctrl.CreateReportTemplate)
		r.PATCH("/templates/:id", permissionMiddleware("report_template:manage"), ctrl.UpdateReportTemplate)
		r.DELETE("/templates/:id", permissionMiddleware("report_template:manage"), ctrl.DeleteReportTemplate)
		r.GET("/workflow-status", permissionMiddleware("exam:view"), ctrl.ListReportWorkflowStatuses)
		r.GET("/stats", permissionMiddleware("exam:view"), ctrl.GetConsolidatedStats)
		r.GET("/:id/pdf-preview", permissionMiddleware("exam:report:view_locked"), ctrl.DownloadReportPreviewPDF)
		r.GET("/:id", permissionMiddleware("exam:view"), ctrl.GetReport)
		r.POST("/:id/finalize", permissionMiddleware("exam:report"), ctrl.FinalizeReport)
		r.POST("/:id/override-unlock", permissionMiddleware("exam:report:override"), ctrl.OverrideUnlockReport)
		r.POST("/documents", permissionMiddleware("document:manage"), ctrl.UploadDocument)
		r.GET("/:id/documents", permissionMiddleware("exam:view"), ctrl.GetDocuments)
		r.POST("/shares", permissionMiddleware("exam:view"), ctrl.CreateSecureShare)
		r.GET("/shares", permissionMiddleware("exam:view"), ctrl.ListSecureShares)
		r.POST("/shares/:id/regenerate", permissionMiddleware("exam:view"), ctrl.RegenerateSecureShare)
		r.POST("/shares/:id/archive", permissionMiddleware("exam:view"), ctrl.ArchiveSecureShare)
		r.POST("/shares/:id/restore", permissionMiddleware("exam:view"), ctrl.RestoreSecureShare)
	}
}

// RegisterPublicRoutes mounts public secure share unlocking endpoint.
func RegisterPublicRoutes(router *gin.RouterGroup, ctrl *Controller) {
	p := router.Group("/shared-reports")
	{
		p.GET("/:token", ctrl.GetSecureShare)
	}
	v := router.Group("/report-verification")
	{
		v.GET("/:code", ctrl.GetReportVerification)
		v.POST("/:code", ctrl.VerifyReportPDF)
	}
}
