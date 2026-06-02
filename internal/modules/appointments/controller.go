package appointments

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"my-app/internal/middleware"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

// restrictToExaminer returns the examiner's user id when the caller only has
// examiner-level access (no client:manage) and may therefore only touch records tied
// to their own appointments. Returns (0, false) when the caller has full access.
func restrictToExaminer(c *gin.Context) (uint, bool) {
	if middleware.HasPermission(c, "client:manage") {
		return 0, false
	}
	if uid, ok := c.Get("user_id"); ok {
		if id, ok := uid.(uint); ok && id > 0 {
			return id, true
		}
	}
	return 0, true // restricted but no resolvable id → deny
}

// hideFinancials zeroes out monetary fields for callers without payment:view (e.g.
// examiners). Payment status (Paid/Partial/Unpaid) is preserved — only amounts and
// payment mode are hidden.
func hideFinancials(c *gin.Context, appointments []Appointment) []Appointment {
	if middleware.HasPermission(c, "payment:view") {
		return appointments
	}
	for i := range appointments {
		appointments[i].ExamFee = 0
		appointments[i].CollectedAmount = 0
		appointments[i].PaymentMode = ""
	}
	return appointments
}

// CreateClient godoc
// @Summary Register a new client
// @Tags business
// @Accept json
// @Produce json
// @Param client body Client true "Client Data"
// @Success 201 {object} Client
// @Router /api/clients [post]
func (ctrl *Controller) CreateClient(c *gin.Context) {
	var client Client
	if err := c.ShouldBindJSON(&client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.service.CreateClient(&client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create client"})
		return
	}
	c.JSON(http.StatusCreated, client)
}

// GetClients godoc
// @Summary Get all clients
// @Tags business
// @Produce json
// @Success 200 {array} Client
// @Router /api/clients [get]
func (ctrl *Controller) GetClients(c *gin.Context) {
	var (
		clients []Client
		err     error
	)
	if examinerID, restricted := restrictToExaminer(c); restricted {
		clients, err = ctrl.service.GetClientsForExaminer(examinerID, c.Query("search"))
	} else {
		clients, err = ctrl.service.GetAllClients(c.Query("search"))
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch clients"})
		return
	}
	c.JSON(http.StatusOK, clients)
}

func (ctrl *Controller) GetClient(c *gin.Context) {
	if examinerID, restricted := restrictToExaminer(c); restricted {
		if !ctrl.service.ExaminerOwnsClient(examinerID, c.Param("id")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have access to this client"})
			return
		}
	}
	client, err := ctrl.service.GetClientByID(c.Param("id"))
	if err != nil {
		if err.Error() == "client not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch client"})
		return
	}
	c.JSON(http.StatusOK, client)
}

func (ctrl *Controller) DeleteClient(c *gin.Context) {
	var body struct {
		ConfirmName string `json:"confirm_name"`
	}
	_ = c.ShouldBindJSON(&body)

	if err := ctrl.service.DeleteClient(c.Param("id"), body.ConfirmName); err != nil {
		switch err.Error() {
		case "client not found":
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case "confirmation name does not match client name", "invalid client id":
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete client"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "client deleted"})
}

func (ctrl *Controller) UpdateClient(c *gin.Context) {
	var input Client
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.service.UpdateClient(c.Param("id"), &input); err != nil {
		if err.Error() == "client not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client, err := ctrl.service.GetClientByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch updated client"})
		return
	}
	c.JSON(http.StatusOK, client)
}

func (ctrl *Controller) GetClientExaminees(c *gin.Context) {
	if examinerID, restricted := restrictToExaminer(c); restricted {
		if !ctrl.service.ExaminerOwnsClient(examinerID, c.Param("id")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have access to this client"})
			return
		}
	}
	entries, err := ctrl.service.GetClientExaminees(c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "client not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entries)
}

func (ctrl *Controller) BulkCreateExaminees(c *gin.Context) {
	var input struct {
		Examinees []struct {
			FirstName   string `json:"first_name"`
			LastName    string `json:"last_name"`
			Email       string `json:"email"`
			Phone       string `json:"phone"`
			EmployeeRef string `json:"employee_ref"`
		} `json:"examinees" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rows := make([]bulkExamineeInput, len(input.Examinees))
	for i, e := range input.Examinees {
		rows[i] = bulkExamineeInput{
			FirstName:   e.FirstName,
			LastName:    e.LastName,
			Email:       e.Email,
			Phone:       e.Phone,
			EmployeeRef: e.EmployeeRef,
		}
	}

	created, err := ctrl.service.BulkCreateExaminees(c.Param("id"), rows)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"created": len(created),
		"examinees": created,
	})
}

func (ctrl *Controller) GetSubjectAppointments(c *gin.Context) {
	if examinerID, restricted := restrictToExaminer(c); restricted {
		if !ctrl.service.ExaminerOwnsSubject(examinerID, c.Param("id")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have access to this examinee"})
			return
		}
	}
	appointments, err := ctrl.service.GetSubjectAppointments(c.Param("id"), c.Query("client_id"))
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "subject not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, hideFinancials(c, appointments))
}

func (ctrl *Controller) GetClientAppointments(c *gin.Context) {
	if examinerID, restricted := restrictToExaminer(c); restricted {
		if !ctrl.service.ExaminerOwnsClient(examinerID, c.Param("id")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have access to this client"})
			return
		}
	}
	appointments, err := ctrl.service.GetAllAppointments(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}
	c.JSON(http.StatusOK, hideFinancials(c, appointments))
}

func (ctrl *Controller) GetClientDocuments(c *gin.Context) {
	if examinerID, restricted := restrictToExaminer(c); restricted {
		if !ctrl.service.ExaminerOwnsClient(examinerID, c.Param("id")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have access to this client"})
			return
		}
	}
	docs, err := ctrl.service.GetClientDocuments(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch documents"})
		return
	}
	c.JSON(http.StatusOK, docs)
}

func (ctrl *Controller) UploadClientDocument(c *gin.Context) {
	clientID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	docType := c.PostForm("type")
	source := c.PostForm("source")

	doc, err := ctrl.service.UploadClientDocument(
		c.Request.Context(),
		uint(clientID),
		header.Filename,
		docType,
		source,
		file,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, doc)
}

func (ctrl *Controller) CreateClientFormDocument(c *gin.Context) {
	clientID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
		return
	}

	var input struct {
		Name     string                 `json:"name"`
		Type     string                 `json:"type"`
		FormData map[string]interface{} `json:"form_data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	doc, err := ctrl.service.CreateClientFormDocument(uint(clientID), input.Name, input.Type, input.FormData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, doc)
}

func (ctrl *Controller) GetSubjectDocuments(c *gin.Context) {
	docs, err := ctrl.service.GetSubjectDocuments(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch documents"})
		return
	}
	c.JSON(http.StatusOK, docs)
}

func (ctrl *Controller) UploadSubjectDocument(c *gin.Context) {
	subjectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subject id"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	docType := c.PostForm("type")
	source := c.PostForm("source")

	doc, err := ctrl.service.UploadSubjectDocument(
		c.Request.Context(),
		uint(subjectID),
		header.Filename,
		docType,
		source,
		file,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, doc)
}

// CreateAppointment godoc
// @Summary Schedule a new appointment
// @Tags business
// @Accept json
// @Produce json
// @Param appointment body Appointment true "Appointment Data"
// @Success 201 {object} Appointment
// @Router /api/appointments [post]
func (ctrl *Controller) CreateAppointment(c *gin.Context) {
	var appointment Appointment
	if err := c.ShouldBindJSON(&appointment); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.service.CreateAppointment(&appointment); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, appointment)
}

// GetAppointments godoc
// @Summary Get all appointments
// @Tags business
// @Produce json
// @Success 200 {array} Appointment
// @Router /api/appointments [get]
func (ctrl *Controller) GetAppointments(c *gin.Context) {
	appointments, err := ctrl.service.GetAllAppointments(c.Query("client_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointments"})
		return
	}
	c.JSON(http.StatusOK, hideFinancials(c, appointments))
}

func (ctrl *Controller) GetAppointment(c *gin.Context) {
	if examinerID, restricted := restrictToExaminer(c); restricted {
		if !ctrl.service.ExaminerOwnsAppointment(examinerID, c.Param("id")) {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have access to this appointment"})
			return
		}
	}
	appointment, err := ctrl.service.GetAppointmentByID(c.Param("id"))
	if err != nil {
		if err.Error() == "appointment not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch appointment"})
		return
	}
	if !middleware.HasPermission(c, "payment:view") {
		appointment.ExamFee = 0
		appointment.CollectedAmount = 0
		appointment.PaymentMode = ""
	}
	c.JSON(http.StatusOK, appointment)
}

func (ctrl *Controller) UpdateAppointment(c *gin.Context) {
	var input map[string]interface{}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	appointment, err := ctrl.service.UpdateAppointment(c.Param("id"), input)
	if err != nil {
		if err.Error() == "appointment not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, appointment)
}

func (ctrl *Controller) DeleteAppointment(c *gin.Context) {
	if err := ctrl.service.DeleteAppointment(c.Param("id")); err != nil {
		if err.Error() == "appointment not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete appointment"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "appointment deleted"})
}

// UpdateStatus godoc
// @Summary Update appointment status
// @Tags business
// @Accept json
// @Produce json
// @Param id path int true "Appointment ID"
// @Param status body map[string]string true "Status"
// @Success 200 {object} map[string]string
// @Router /api/appointments/{id}/status [patch]
func (ctrl *Controller) UpdateStatus(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.service.UpdateStatus(id, input.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}

// UpdatePayment godoc
// @Summary Update appointment payment fields
// @Tags business
// @Accept json
// @Produce json
// @Param id path int true "Appointment ID"
// @Param payment body map[string]interface{} true "Payment fields"
// @Success 200 {object} map[string]string
// @Router /api/appointments/{id}/payment [patch]
func (ctrl *Controller) GetBillingLedger(c *gin.Context) {
	entries, summary, err := ctrl.service.GetBillingLedger(c.Query("client_id"))
	if err != nil {
		if err.Error() == "client not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch billing ledger"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"summary": summary,
		"entries": entries,
	})
}

func (ctrl *Controller) GetClientAccount(c *gin.Context) {
	entries, summary, err := ctrl.service.GetClientAccount(c.Param("id"))
	if err != nil {
		if err.Error() == "client not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch client account"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"summary": summary,
		"entries": entries,
	})
}

func (ctrl *Controller) CollectAppointmentPayment(c *gin.Context) {
	var input struct {
		Amount float64 `json:"amount" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	appt, err := ctrl.service.CollectAppointmentPayment(c.Param("id"), input.Amount)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "appointment not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, appt)
}

func (ctrl *Controller) SendAppointmentPaymentReminder(c *gin.Context) {
	var input struct {
		ToEmail string `json:"to_email"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.service.SendAppointmentPaymentReminder(c.Param("id"), input.ToEmail, input.Subject, input.Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Payment reminder sent"})
}

func (ctrl *Controller) UpdatePayment(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		PaymentStatus string   `json:"payment_status" binding:"required"`
		ExamFee       *float64 `json:"exam_fee"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.service.UpdatePayment(id, input.PaymentStatus, input.ExamFee); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Payment updated"})
}

// GetQuotations godoc
// @Summary Get all quotations
// @Tags business
// @Produce json
// @Success 200 {array} Quotation
// @Router /api/quotations [get]
func (ctrl *Controller) GetQuotations(c *gin.Context) {
	quotes, err := ctrl.service.GetAllQuotations(c.Query("search"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch quotations"})
		return
	}
	c.JSON(http.StatusOK, quotes)
}

// CreateQuotation godoc
// @Summary Create quotation
// @Tags business
// @Accept json
// @Produce json
// @Param quotation body Quotation true "Quotation Data"
// @Success 201 {object} Quotation
// @Router /api/quotations [post]
func (ctrl *Controller) CreateQuotation(c *gin.Context) {
	var quote Quotation
	if err := c.ShouldBindJSON(&quote); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ctrl.service.CreateQuotation(&quote); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, quote)
}

// SendQuotationEmail godoc
// @Summary Mark quotation as emailed
// @Tags business
// @Accept json
// @Produce json
// @Param id path int true "Quotation ID"
// @Param email body map[string]string true "Email payload"
// @Success 200 {object} map[string]string
// @Router /api/quotations/{id}/send-email [patch]
func (ctrl *Controller) SendQuotationEmail(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		ToEmail string `json:"to_email" binding:"required"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.service.MarkQuotationSent(id, input.ToEmail, input.Subject, input.Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Quotation marked as emailed"})
}

// CollectQuotationPayment godoc
// @Summary Collect quotation payment
// @Tags business
// @Accept json
// @Produce json
// @Param id path int true "Quotation ID"
// @Param payment body map[string]float64 true "Payment amount"
// @Success 200 {object} map[string]string
// @Router /api/quotations/{id}/collect-payment [patch]
func (ctrl *Controller) CollectQuotationPayment(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		Amount float64 `json:"amount" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ctrl.service.CollectQuotationPayment(id, input.Amount); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Payment collected"})
}

func (ctrl *Controller) ConvertQuotation(c *gin.Context) {
	var input ConvertQuotationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	appt, err := ctrl.service.ConvertQuotationToAppointment(c.Param("id"), input)
	if err != nil {
		if err.Error() == "quotation not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, appt)
}

func (ctrl *Controller) DeleteQuotation(c *gin.Context) {
	if err := ctrl.service.DeleteQuotation(c.Param("id")); err != nil {
		if err.Error() == "quotation not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete invoice"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "invoice deleted"})
}
