package exams

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"my-app/internal/middleware"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxVerificationPDFSize = 20 << 20

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
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "locked forensic report") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
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
	if report.IsLocked && !middleware.HasPermission(c, "exam:report:view_locked") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied: requires exam:report:view_locked"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                 report.ID,
		"exam_id":            report.ExamID,
		"verdict":            report.Verdict,
		"content":            decrypted,
		"created_at":         report.CreatedAt,
		"is_locked":          report.IsLocked,
		"locked_at":          report.LockedAt,
		"signature_examiner": report.SignatureExaminer,
		"signature_client":   report.SignatureClient,
	})
}

func (ctrl *Controller) FinalizeReport(c *gin.Context) {
	examID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report id"})
		return
	}

	userVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID, ok := userVal.(uint)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	actorEmail, _ := c.Get("email")
	emailStr, _ := actorEmail.(string)
	var input struct {
		ExaminerID             uint `json:"examiner_id" binding:"required"`
		AuthorizationConfirmed bool `json:"authorization_confirmed" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || !input.AuthorizationConfirmed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Examiner authorization confirmation is required"})
		return
	}

	report, finalizeErr := ctrl.service.FinalizeReport(uint(examID), userID, emailStr, input.ExaminerID)
	if finalizeErr != nil {
		status := http.StatusBadRequest
		if strings.Contains(finalizeErr.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(finalizeErr.Error(), "already locked") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": finalizeErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                 report.ID,
		"exam_id":            report.ExamID,
		"is_locked":          report.IsLocked,
		"locked_at":          report.LockedAt,
		"signature_examiner": report.SignatureExaminer,
		"signer_name":        report.SignerName,
		"signed_at":          report.SignedAt,
	})
}

func (ctrl *Controller) OverrideUnlockReport(c *gin.Context) {
	examID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid report id"})
		return
	}

	var input struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID, ok := userVal.(uint)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	report, unlockErr := ctrl.service.UnlockReportForRevision(uint(examID), userID, input.Reason)
	if unlockErr != nil {
		status := http.StatusBadRequest
		if strings.Contains(unlockErr.Error(), "not found") {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": unlockErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":        report.ID,
		"exam_id":   report.ExamID,
		"is_locked": report.IsLocked,
		"locked_at": report.LockedAt,
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

func (ctrl *Controller) ListSecureShares(c *gin.Context) {
	search := c.Query("search")
	clientIDStr := c.Query("client_id")
	subjectIDStr := c.Query("subject_id")

	var clientID uint
	if clientIDStr != "" {
		id, err := strconv.ParseUint(clientIDStr, 10, 32)
		if err == nil {
			clientID = uint(id)
		}
	}

	var subjectID uint
	if subjectIDStr != "" {
		id, err := strconv.ParseUint(subjectIDStr, 10, 32)
		if err == nil {
			subjectID = uint(id)
		}
	}

	var examinerID uint
	if !middleware.HasPermission(c, "client:manage") {
		if uid, ok := c.Get("user_id"); ok {
			examinerID, _ = uid.(uint)
		}
	}
	shares, err := ctrl.service.ListSecureShares(search, clientID, subjectID, examinerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch secure shares"})
		return
	}
	c.JSON(http.StatusOK, shares)
}

func (ctrl *Controller) CreateSecureShare(c *gin.Context) {
	var input struct {
		ExamReportID   uint   `json:"exam_report_id"`
		ExamID         uint   `json:"exam_id"`
		RecipientEmail string `json:"recipient_email"`
		ExpiresInDays  int    `json:"expires_in_days"`
		ProtectionMode string `json:"protection_mode"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.RecipientEmail == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "recipient_email is required"})
		return
	}

	reportID := input.ExamReportID
	if reportID == 0 && input.ExamID > 0 {
		var report ExamReport
		if err := ctrl.service.db.Where("exam_id = ?", input.ExamID).First(&report).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No report has been written for this exam yet"})
			return
		}
		reportID = report.ID
	}

	if reportID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "exam_report_id or exam_id is required"})
		return
	}

	share, err := ctrl.service.CreateSecureShare(reportID, input.RecipientEmail, input.ExpiresInDays, input.ProtectionMode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, share)
}

func (ctrl *Controller) GetSecureShare(c *gin.Context) {
	token := c.Param("token")
	share, err := ctrl.service.GetSecureReportShareByToken(token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Shared report not found or expired"})
		return
	}
	c.JSON(http.StatusOK, share)
}

func (ctrl *Controller) GetReportVerification(c *gin.Context) {
	share, err := ctrl.service.GetReportVerification(c.Param("code"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"valid": false, "error": "Verification record not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"valid":             true,
		"verification_code": share.VerificationCode,
		"issued_at":         share.CreatedAt,
		"report_locked":     share.ExamReport != nil && share.ExamReport.IsLocked,
	})
}

func (ctrl *Controller) VerifyReportPDF(c *gin.Context) {
	share, err := ctrl.service.GetReportVerification(c.Param("code"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"valid": false, "error": "Verification record not found"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxVerificationPDFSize+(1<<20))
	file, err := c.FormFile("file")
	if err != nil || file.Size <= 0 || file.Size > maxVerificationPDFSize {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "A PDF up to 20 MB is required"})
		return
	}
	opened, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "Unable to read PDF"})
		return
	}
	defer opened.Close()
	hasher := sha256.New()
	buffer := make([]byte, 512)
	n, _ := opened.Read(buffer)
	if n < 5 || string(buffer[:5]) != "%PDF-" {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "The uploaded file is not a PDF"})
		return
	}
	hasher.Write(buffer[:n])
	if _, err := io.Copy(hasher, opened); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "Unable to read PDF"})
		return
	}
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	locked := share.ExamReport != nil && share.ExamReport.IsLocked
	hashMatches := share.PDFHash != "" && subtle.ConstantTimeCompare([]byte(actualHash), []byte(share.PDFHash)) == 1
	authentic := hashMatches && locked
	message := "Invalid - this PDF has been modified or was not issued by us"
	if hashMatches && !locked {
		message = "Invalid - this report has been revoked or reopened for revision"
	} else if authentic {
		message = "Authentic - this PDF is unchanged"
	}
	c.JSON(http.StatusOK, gin.H{
		"valid":             authentic,
		"verification_code": share.VerificationCode,
		"issued_at":         share.CreatedAt,
		"report_locked":     locked,
		"message":           message,
	})
}

func (ctrl *Controller) RegenerateSecureShare(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid share ID"})
		return
	}

	var input struct {
		ExpiresInDays  int    `json:"expires_in_days"`
		ProtectionMode string `json:"protection_mode"`
	}
	_ = c.ShouldBindJSON(&input)

	share, err := ctrl.service.RegenerateSecureReportShare(uint(id), input.ExpiresInDays, input.ProtectionMode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, share)
}

func (ctrl *Controller) GetConsolidatedStats(c *gin.Context) {
	var examinerID uint
	if middleware.HasPermission(c, "client:manage") {
		if requested, err := strconv.ParseUint(c.Query("examiner_id"), 10, 64); err == nil {
			examinerID = uint(requested)
		}
	} else if uid, ok := c.Get("user_id"); ok {
		examinerID, _ = uid.(uint)
	}
	stats, err := ctrl.service.GetConsolidatedReportStats(examinerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stats"})
		return
	}
	c.JSON(http.StatusOK, stats)
}
