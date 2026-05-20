package exams

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (ctrl *Controller) CreateExam(c *gin.Context) {
	var exam Exam
	if err := c.ShouldBindJSON(&exam); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.service.CreateExam(&exam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create exam"})
		return
	}
	c.JSON(http.StatusCreated, exam)
}

func (ctrl *Controller) GetAllExams(c *gin.Context) {
	exams, err := ctrl.service.GetAllExams()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch exams"})
		return
	}
	c.JSON(http.StatusOK, exams)
}

func (ctrl *Controller) GetAllExamTypes(c *gin.Context) {
	examTypes, err := ctrl.service.GetAllExamTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch exam types"})
		return
	}
	c.JSON(http.StatusOK, examTypes)
}

func (ctrl *Controller) CreateExamType(c *gin.Context) {
	var examType ExamType
	if err := c.ShouldBindJSON(&examType); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	examType.Name = strings.TrimSpace(examType.Name)
	examType.Description = strings.TrimSpace(examType.Description)
	examType.Category = strings.TrimSpace(examType.Category)
	if err := ctrl.service.CreateExamType(&examType); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, examType)
}

func (ctrl *Controller) UpdateExamType(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exam type id"})
		return
	}

	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if name, ok := payload["name"].(string); ok {
		payload["name"] = strings.TrimSpace(name)
	}
	if description, ok := payload["description"].(string); ok {
		payload["description"] = strings.TrimSpace(description)
	}
	if category, ok := payload["category"].(string); ok {
		payload["category"] = strings.TrimSpace(category)
	}

	examType, updateErr := ctrl.service.UpdateExamType(uint(id), payload)
	if updateErr != nil {
		status := http.StatusBadRequest
		if strings.Contains(updateErr.Error(), "not found") {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": updateErr.Error()})
		return
	}
	c.JSON(http.StatusOK, examType)
}

func (ctrl *Controller) DeleteExamType(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid exam type id"})
		return
	}
	if err := ctrl.service.DeleteExamType(uint(id)); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (ctrl *Controller) CreateReport(c *gin.Context) {
	var input struct {
		ExamID  uint   `json:"exam_id" binding:"required"`
		Verdict string `json:"verdict" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	report, err := ctrl.service.CreateReport(input.ExamID, input.Verdict, input.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create report"})
		return
	}
	c.JSON(http.StatusCreated, report)
}

func (ctrl *Controller) GetReport(c *gin.Context) {
	examID, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	report, decrypted, err := ctrl.service.GetReport(uint(examID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":         report.ID,
		"exam_id":    report.ExamID,
		"verdict":    report.Verdict,
		"content":    decrypted,
		"created_at": report.CreatedAt,
	})
}

func (ctrl *Controller) GetDocuments(c *gin.Context) {
	examID := c.Param("id")
	docs, err := ctrl.service.GetDocuments(examID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch documents"})
		return
	}
	c.JSON(http.StatusOK, docs)
}

// UploadDocument godoc
// @Summary Upload a forensic document
// @Tags forensic
// @Accept multipart/form-data
// @Produce json
// @Param exam_id formData int true "Exam ID"
// @Param type formData string true "Document Type"
// @Param file formData file true "Document File"
// @Success 201 {object} Document
// @Router /api/exams/upload [post]
func (ctrl *Controller) UploadDocument(c *gin.Context) {
	examIDStr := c.PostForm("exam_id")
	docType := c.PostForm("type")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	var examID uint
	fmt.Sscanf(examIDStr, "%d", &examID)

	doc, err := ctrl.service.UploadDocument(c.Request.Context(), examID, header.Filename, docType, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, doc)
}

func (ctrl *Controller) CreateReferral(c *gin.Context) {
	var ref CaseReferral
	if err := c.ShouldBindJSON(&ref); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.service.CreateReferral(&ref); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create referral"})
		return
	}
	c.JSON(http.StatusCreated, ref)
}

func (ctrl *Controller) CreateAssessment(c *gin.Context) {
	var ass ClinicalAssessment
	if err := c.ShouldBindJSON(&ass); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.service.CreateAssessment(&ass); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create assessment"})
		return
	}
	c.JSON(http.StatusCreated, ass)
}

func (ctrl *Controller) AddPhase(c *gin.Context) {
	var phase ExamPhase
	if err := c.ShouldBindJSON(&phase); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.service.AddPhase(&phase); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add phase"})
		return
	}
	c.JSON(http.StatusCreated, phase)
}

func (ctrl *Controller) GetIntelligence(c *gin.Context) {
	examID := c.Param("id")
	exam, err := ctrl.service.GetIntelligence(examID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Intelligence not found"})
		return
	}
	c.JSON(http.StatusOK, exam)
}

func (ctrl *Controller) GetExam(c *gin.Context) {
	exam, err := ctrl.service.GetExamByID(c.Param("id"))
	if err != nil {
		if err.Error() == "exam not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch exam"})
		return
	}
	c.JSON(http.StatusOK, exam)
}

func (ctrl *Controller) GetExamByAppointment(c *gin.Context) {
	exam, err := ctrl.service.GetExamByAppointmentID(c.Param("appointmentId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch exam"})
		return
	}
	if exam == nil {
		c.JSON(http.StatusOK, nil)
		return
	}
	c.JSON(http.StatusOK, exam)
}

func (ctrl *Controller) UpdateExam(c *gin.Context) {
	var input map[string]interface{}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	exam, err := ctrl.service.UpdateExam(c.Param("id"), input)
	if err != nil {
		if err.Error() == "exam not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, exam)
}

func (ctrl *Controller) StartDocumentation(c *gin.Context) {
	exam, err := ctrl.service.StartDocumentationForAppointment(c.Param("appointmentId"))
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "appointment not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, exam)
}
