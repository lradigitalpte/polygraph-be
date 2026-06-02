package appointments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"my-app/internal/database"
	"my-app/internal/modules/auth"
	"my-app/internal/modules/availability"
	"my-app/internal/modules/subjects"
	"my-app/internal/storage"

	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	storage storage.Storage
}

func NewService(store storage.Storage) *Service {
	return &Service{db: database.GetDB(), storage: store}
}

func (s *Service) CreateClient(client *Client) error {
	return s.db.Create(client).Error
}

func (s *Service) GetAllClients(search ...string) ([]Client, error) {
	var clients []Client
	query := s.db.Order("name ASC")
	filter := ""
	if len(search) > 0 {
		filter = search[0]
	}
	if trimmed := strings.TrimSpace(strings.ToLower(filter)); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where("LOWER(name) LIKE ? OR LOWER(email) LIKE ? OR LOWER(contact_person) LIKE ?", like, like, like)
	}
	err := query.Find(&clients).Error
	return clients, err
}

func (s *Service) GetClientByID(id string) (*Client, error) {
	var client Client
	if err := s.db.First(&client, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("client not found")
		}
		return nil, err
	}
	return &client, nil
}

// GetClientsForExaminer returns only clients the examiner has appointments with.
func (s *Service) GetClientsForExaminer(examinerID uint, search ...string) ([]Client, error) {
	var clients []Client
	owned := s.db.Model(&Appointment{}).Select("client_id").Where("examiner_id = ?", examinerID)
	query := s.db.Order("name ASC").Where("id IN (?)", owned)
	if len(search) > 0 {
		if trimmed := strings.TrimSpace(strings.ToLower(search[0])); trimmed != "" {
			like := "%" + trimmed + "%"
			query = query.Where("LOWER(name) LIKE ? OR LOWER(email) LIKE ? OR LOWER(contact_person) LIKE ?", like, like, like)
		}
	}
	err := query.Find(&clients).Error
	return clients, err
}

// Ownership checks — an examiner "owns" a client/appointment/subject when they have
// at least one appointment linking them to it.
func (s *Service) ExaminerOwnsClient(examinerID uint, clientID string) bool {
	var count int64
	s.db.Model(&Appointment{}).Where("examiner_id = ? AND client_id = ?", examinerID, clientID).Count(&count)
	return count > 0
}

func (s *Service) ExaminerOwnsAppointment(examinerID uint, appointmentID string) bool {
	var count int64
	s.db.Model(&Appointment{}).Where("examiner_id = ? AND id = ?", examinerID, appointmentID).Count(&count)
	return count > 0
}

func (s *Service) ExaminerOwnsSubject(examinerID uint, subjectID string) bool {
	var count int64
	s.db.Model(&Appointment{}).Where("examiner_id = ? AND subject_id = ?", examinerID, subjectID).Count(&count)
	return count > 0
}

func (s *Service) UpdateClient(id string, input *Client) error {
	var client Client
	if err := s.db.First(&client, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("client not found")
		}
		return err
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)
	if input.Name == "" {
		return errors.New("name is required")
	}
	if input.Email == "" {
		return errors.New("email is required")
	}
	if input.ClientType == "" {
		input.ClientType = client.ClientType
	}
	if input.PreferredPaymentMethod == "" {
		input.PreferredPaymentMethod = client.PreferredPaymentMethod
	}

	updates := map[string]interface{}{
		"name":                     input.Name,
		"client_type":              input.ClientType,
		"gender":                   strings.TrimSpace(input.Gender),
		"contact_person":           strings.TrimSpace(input.ContactPerson),
		"phone":                    strings.TrimSpace(input.Phone),
		"email":                    input.Email,
		"address":                  strings.TrimSpace(input.Address),
		"tax_id":                   strings.TrimSpace(input.TaxID),
		"preferred_payment_method": input.PreferredPaymentMethod,
		"notes":                    strings.TrimSpace(input.Notes),
	}

	return s.db.Model(&client).Updates(updates).Error
}

func (s *Service) CreateAppointment(app *Appointment) error {
	if err := s.validateAppointment(app); err != nil {
		return err
	}
	if err := s.db.Create(app).Error; err != nil {
		return err
	}
	return s.ensureInvoiceForAppointment(app)
}

func (s *Service) GetAllAppointments(clientID ...string) ([]Appointment, error) {
	var appointments []Appointment
	query := s.db.Preload("Client").Order("scheduled_at DESC")
	if len(clientID) > 0 && strings.TrimSpace(clientID[0]) != "" {
		query = query.Where("client_id = ?", strings.TrimSpace(clientID[0]))
	}
	err := query.Find(&appointments).Error
	return appointments, err
}

func (s *Service) GetClientDocuments(clientID string) ([]ClientDocument, error) {
	var docs []ClientDocument
	err := s.db.Where("client_id = ?", clientID).Order("created_at DESC").Find(&docs).Error
	return docs, err
}

func (s *Service) UploadClientDocument(ctx context.Context, clientID uint, fileName, docType, source string, body io.Reader) (*ClientDocument, error) {
	if _, err := s.GetClientByID(strconv.FormatUint(uint64(clientID), 10)); err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, errors.New("file storage is not configured")
	}

	docType = strings.TrimSpace(docType)
	if docType == "" {
		docType = "upload"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "upload"
	}

	fileBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(fileBytes)
	hashSum := hex.EncodeToString(hasher.Sum(nil))

	key := fmt.Sprintf("clients/%d/%s", clientID, fileName)
	url, err := s.storage.UploadFile(ctx, key, bytes.NewReader(fileBytes), "application/octet-stream")
	if err != nil {
		return nil, err
	}

	doc := ClientDocument{
		ClientID: clientID,
		Name:     fileName,
		Type:     docType,
		Source:   source,
		URL:      url,
		Hash:     hashSum,
	}
	if err := s.db.Create(&doc).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *Service) CreateClientFormDocument(clientID uint, name, docType string, formData map[string]interface{}) (*ClientDocument, error) {
	if _, err := s.GetClientByID(strconv.FormatUint(uint64(clientID), 10)); err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = "Online form submission"
	}
	docType = strings.TrimSpace(docType)
	if docType == "" {
		docType = "intake_form"
	}

	payload, err := json.Marshal(formData)
	if err != nil {
		return nil, fmt.Errorf("invalid form data: %w", err)
	}

	doc := ClientDocument{
		ClientID: clientID,
		Name:     name,
		Type:     docType,
		Source:   "online_form",
		FormData: string(payload),
	}
	if err := s.db.Create(&doc).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *Service) GetSubjectDocuments(subjectID string) ([]SubjectDocument, error) {
	var docs []SubjectDocument
	err := s.db.Where("subject_id = ?", subjectID).Order("created_at DESC").Find(&docs).Error
	return docs, err
}

func (s *Service) UploadSubjectDocument(ctx context.Context, subjectID uint, fileName, docType, source string, body io.Reader) (*SubjectDocument, error) {
	subjectSvc := subjects.NewService()
	subject, err := subjectSvc.GetByID(strconv.FormatUint(uint64(subjectID), 10))
	if err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, errors.New("file storage is not configured")
	}

	docType = strings.TrimSpace(docType)
	if docType == "" {
		docType = "upload"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "upload"
	}

	fileBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(fileBytes)
	hashSum := hex.EncodeToString(hasher.Sum(nil))

	clientID := uint(0)
	if subject.ClientID != nil {
		clientID = *subject.ClientID
	}

	key := fmt.Sprintf("subjects/%d/%s", subjectID, fileName)
	url, err := s.storage.UploadFile(ctx, key, bytes.NewReader(fileBytes), "application/octet-stream")
	if err != nil {
		return nil, err
	}

	doc := SubjectDocument{
		SubjectID: subjectID,
		ClientID:  clientID,
		Name:      fileName,
		Type:      docType,
		Source:    source,
		URL:       url,
		Hash:      hashSum,
	}
	if err := s.db.Create(&doc).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *Service) GetAppointmentByID(id string) (*Appointment, error) {
	var appointment Appointment
	if err := s.db.Preload("Client").First(&appointment, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("appointment not found")
		}
		return nil, err
	}
	return &appointment, nil
}

func (s *Service) UpdateAppointment(id string, updates map[string]interface{}) (*Appointment, error) {
	var appointment Appointment
	if err := s.db.First(&appointment, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("appointment not found")
		}
		return nil, err
	}

	allowed := map[string]bool{
		"notes": true, "status": true, "questions_prepared": true, "exam_id": true,
		"scheduled_at": true, "duration": true, "examiner_id": true,
	}
	safe := make(map[string]interface{})
	for key, value := range updates {
		if allowed[key] {
			safe[key] = value
		}
	}
	if len(safe) == 0 {
		return &appointment, nil
	}

	// Reschedule path — validate the new slot against Sundays, examiner blocks, and
	// other appointments (excluding this one) before persisting.
	rescheduled := false
	newScheduledAt := appointment.ScheduledAt
	newDuration := appointment.Duration
	newExaminer := appointment.ExaminerID

	if v, ok := safe["scheduled_at"].(string); ok && v != "" {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New("scheduled_at must be an RFC3339 timestamp")
		}
		if parsed.UTC().Weekday() == time.Sunday {
			return nil, errors.New("appointments cannot be scheduled on Sunday")
		}
		newScheduledAt = parsed
		safe["scheduled_at"] = parsed
		rescheduled = true
	}
	if v, ok := safe["duration"].(float64); ok {
		newDuration = int(v)
	}
	if v, ok := safe["examiner_id"].(float64); ok {
		newExaminer = uint(v)
		rescheduled = true
	}

	if rescheduled {
		if blocked, err := s.hasAvailabilityConflict(newExaminer, newScheduledAt, newDuration); err != nil {
			return nil, err
		} else if blocked {
			return nil, errors.New("selected slot conflicts with examiner availability")
		}
		if conflict, err := s.hasAppointmentConflictExcluding(newExaminer, newScheduledAt, newDuration, appointment.ID); err != nil {
			return nil, err
		} else if conflict {
			return nil, errors.New("selected slot overlaps an existing appointment")
		}
	}

	if err := s.db.Model(&appointment).Updates(safe).Error; err != nil {
		return nil, err
	}
	if err := s.db.Preload("Client").First(&appointment, id).Error; err != nil {
		return nil, err
	}
	return &appointment, nil
}

func (s *Service) DeleteAppointment(id string) error {
	result := s.db.Delete(&Appointment{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("appointment not found")
	}
	return nil
}

func (s *Service) UpdateStatus(id string, status string) error {
	return s.db.Model(&Appointment{}).Where("id = ?", id).Update("status", status).Error
}

func (s *Service) CollectAppointmentPayment(id string, amount float64) (*Appointment, error) {
	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	var appt Appointment
	if err := s.db.Preload("Client").First(&appt, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("appointment not found")
		}
		return nil, err
	}

	newCollected := appt.CollectedAmount + amount
	totalDue := appt.ExamFee
	if totalDue <= 0 {
		totalDue = newCollected
	}
	if newCollected > totalDue {
		newCollected = totalDue
	}

	status := "Partial"
	switch {
	case newCollected <= 0:
		status = "Unpaid"
	case newCollected >= totalDue && totalDue > 0:
		status = "Paid"
	default:
		status = "Partial"
	}

	if err := s.db.Model(&appt).Updates(map[string]interface{}{
		"collected_amount": newCollected,
		"payment_status":   status,
	}).Error; err != nil {
		return nil, err
	}

	_ = s.syncQuotationFromAppointment(appt.ID)

	return s.GetAppointmentByID(id)
}

func (s *Service) SendAppointmentPaymentReminder(id, toEmail, subject, body string) error {
	var appt Appointment
	if err := s.db.Preload("Client").First(&appt, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("appointment not found")
		}
		return err
	}

	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" {
		if strings.TrimSpace(appt.Client.Email) != "" {
			toEmail = appt.Client.Email
		}
	}
	if toEmail == "" || !strings.Contains(toEmail, "@") {
		return errors.New("valid to_email is required")
	}

	balance := appt.ExamFee - appt.CollectedAmount
	if balance < 0 {
		balance = 0
	}

	trimmedSubject := strings.TrimSpace(subject)
	if trimmedSubject == "" {
		trimmedSubject = fmt.Sprintf("Payment reminder — %s", formatAppointmentCode(appt.ID))
	}
	trimmedBody := strings.TrimSpace(body)
	if trimmedBody == "" {
		trimmedBody = fmt.Sprintf(
			"Hello,\n\nThis is a reminder regarding your scheduled polygraph session (%s).\n\nTotal fee: $%.2f\nPaid to date: $%.2f\nBalance due: $%.2f\n\nPlease contact us to arrange payment.\n\nThank you,\nPolygraph Forensic System",
			formatAppointmentCode(appt.ID),
			appt.ExamFee,
			appt.CollectedAmount,
			balance,
		)
	}

	return sendSMTPMail(toEmail, trimmedSubject, trimmedBody)
}

func formatAppointmentCode(id uint) string {
	return fmt.Sprintf("APT-%04d", id)
}

func (s *Service) GetClientAccount(clientID string) ([]AccountLedgerEntry, AccountSummary, error) {
	var client Client
	if err := s.db.First(&client, clientID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, AccountSummary{}, errors.New("client not found")
		}
		return nil, AccountSummary{}, err
	}
	return s.buildBillingLedger(clientID)
}

func (s *Service) GetBillingLedger(clientID string) ([]AccountLedgerEntry, AccountSummary, error) {
	if trimmed := strings.TrimSpace(clientID); trimmed != "" {
		var client Client
		if err := s.db.First(&client, trimmed).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, AccountSummary{}, errors.New("client not found")
			}
			return nil, AccountSummary{}, err
		}
	}
	return s.buildBillingLedger(clientID)
}

func (s *Service) UpdatePayment(id string, paymentStatus string, examFee *float64) error {
	paymentStatus = strings.TrimSpace(paymentStatus)
	if paymentStatus == "" {
		return errors.New("payment_status is required")
	}

	updates := map[string]interface{}{
		"payment_status": paymentStatus,
	}
	if examFee != nil {
		if *examFee < 0 {
			return errors.New("exam_fee cannot be negative")
		}
		updates["exam_fee"] = *examFee
	}

	return s.db.Model(&Appointment{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Service) validateAppointment(app *Appointment) error {
	if app.ClientID == 0 || app.SubjectID == 0 || app.ExaminerID == 0 {
		return errors.New("client_id, subject_id, and examiner_id are required")
	}
	if app.ScheduledAt.IsZero() {
		return errors.New("scheduled_at is required")
	}
	if app.Duration <= 0 {
		app.Duration = 150
	}
	if app.ExamFee < 0 {
		return errors.New("exam_fee cannot be negative")
	}
	if time.Until(app.ScheduledAt) <= 0 {
		return errors.New("scheduled_at must be in the future")
	}
	if app.ScheduledAt.UTC().Weekday() == time.Sunday {
		return errors.New("appointments cannot be scheduled on Sunday")
	}

	var client Client
	if err := s.db.First(&client, app.ClientID).Error; err != nil {
		return errors.New("client not found")
	}

	var subject subjects.Subject
	if err := s.db.First(&subject, app.SubjectID).Error; err != nil {
		return errors.New("subject not found")
	}

	var examiner auth.User
	if err := s.db.Joins("Role").Where("users.id = ?", app.ExaminerID).First(&examiner).Error; err != nil {
		return errors.New("examiner not found")
	}
	if examiner.Role.Name != "Examiner" {
		return errors.New("selected user is not an examiner")
	}
	status := strings.ToLower(strings.TrimSpace(examiner.Status))
	if status != "active" && status != "pending" {
		return errors.New("examiner is not available for booking")
	}

	blocked, err := s.hasAvailabilityConflict(app.ExaminerID, app.ScheduledAt, app.Duration)
	if err != nil {
		return err
	}
	if blocked {
		return errors.New("selected slot conflicts with examiner availability")
	}

	conflict, err := s.hasAppointmentConflict(app.ExaminerID, app.ScheduledAt, app.Duration)
	if err != nil {
		return err
	}
	if conflict {
		return errors.New("selected slot overlaps an existing appointment")
	}

	if app.Status == "" {
		app.Status = "pending"
	}
	if app.PaymentStatus == "" {
		app.PaymentStatus = "None"
	}
	return nil
}

func (s *Service) hasAvailabilityConflict(examinerID uint, scheduledAt time.Time, duration int) (bool, error) {
	dayStart := scheduledAt.UTC().Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	var blocks []availability.Block
	if err := s.db.Where("examiner_id = ? AND date >= ? AND date < ?", examinerID, dayStart, dayEnd).Find(&blocks).Error; err != nil {
		return false, err
	}
	if len(blocks) == 0 {
		return false, nil
	}

	startMinutes := scheduledAt.UTC().Hour()*60 + scheduledAt.UTC().Minute()
	endMinutes := startMinutes + duration
	for _, block := range blocks {
		if block.IsFullDay {
			return true, nil
		}
		blockStart, err := parseClock(block.StartTime)
		if err != nil {
			return false, fmt.Errorf("invalid availability start time: %w", err)
		}
		blockEnd, err := parseClock(block.EndTime)
		if err != nil {
			return false, fmt.Errorf("invalid availability end time: %w", err)
		}
		if startMinutes < blockEnd && endMinutes > blockStart {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) hasAppointmentConflict(examinerID uint, scheduledAt time.Time, duration int) (bool, error) {
	return s.hasAppointmentConflictExcluding(examinerID, scheduledAt, duration, 0)
}

// hasAppointmentConflictExcluding is the overlap check, ignoring one appointment id
// (used when rescheduling so an appointment doesn't conflict with itself).
func (s *Service) hasAppointmentConflictExcluding(examinerID uint, scheduledAt time.Time, duration int, excludeID uint) (bool, error) {
	var existing []Appointment
	windowStart := scheduledAt.Add(-24 * time.Hour)
	windowEnd := scheduledAt.Add(24 * time.Hour)
	query := s.db.
		Where("examiner_id = ? AND status <> ? AND scheduled_at >= ? AND scheduled_at <= ?", examinerID, "cancelled", windowStart, windowEnd)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Find(&existing).Error; err != nil {
		return false, err
	}

	start := scheduledAt.UTC()
	end := start.Add(time.Duration(duration) * time.Minute)
	for _, appointment := range existing {
		existingStart := appointment.ScheduledAt.UTC()
		existingEnd := existingStart.Add(time.Duration(appointment.Duration) * time.Minute)
		if start.Before(existingEnd) && end.After(existingStart) {
			return true, nil
		}
	}
	return false, nil
}

func parseClock(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, err
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func (s *Service) GetAllQuotations(search string) ([]Quotation, error) {
	var quotes []Quotation
	query := s.db.Preload("Client").Order("created_at DESC")
	trimmed := strings.TrimSpace(strings.ToLower(search))
	if trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where("LOWER(code) LIKE ? OR LOWER(title) LIKE ? OR LOWER(status) LIKE ?", like, like, like)
	}
	err := query.Find(&quotes).Error
	return quotes, err
}

func (s *Service) CreateQuotation(input *Quotation) error {
	if input.ClientID == 0 {
		return errors.New("client_id is required")
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return errors.New("title is required")
	}
	if input.Amount < 0 {
		return errors.New("amount cannot be negative")
	}

	var client Client
	if err := s.db.First(&client, input.ClientID).Error; err != nil {
		return errors.New("client not found")
	}

	if input.Status == "" {
		input.Status = "Pending"
	}

	if err := s.db.Create(input).Error; err != nil {
		return err
	}

	if input.Code == "" {
		input.Code = "INV-" + strconv.Itoa(9000+int(input.ID))
		if err := s.db.Model(&Quotation{}).Where("id = ?", input.ID).Update("code", input.Code).Error; err != nil {
			return err
		}
	}

	return s.db.Preload("Client").First(input, input.ID).Error
}

func (s *Service) MarkQuotationSent(id string, toEmail string, subject string, body string) error {
	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" || !strings.Contains(toEmail, "@") {
		return errors.New("valid to_email is required")
	}

	trimmedSubject := strings.TrimSpace(subject)
	if trimmedSubject == "" {
		trimmedSubject = "Your polygraph quotation"
	}
	trimmedBody := strings.TrimSpace(body)
	if trimmedBody == "" {
		trimmedBody = "Please review your quotation from Polygraph."
	}

	if err := sendSMTPMail(toEmail, trimmedSubject, trimmedBody); err != nil {
		return err
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"sent_to_email": toEmail,
		"email_subject": trimmedSubject,
		"email_body":    trimmedBody,
		"sent_at":       now,
		"status":        "Sent",
	}

	return s.db.Model(&Quotation{}).Where("id = ?", id).Updates(updates).Error
}

func sendSMTPMail(toEmail string, subject string, body string) error {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if host == "" || port == "" {
		return errors.New("SMTP_HOST and SMTP_PORT must be configured")
	}

	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if from == "" {
		from = "noreply@polygraph.local"
	}

	addr := host + ":" + port
	message := []byte("Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n" +
		body + "\r\n")

	user := strings.TrimSpace(os.Getenv("SMTP_USER"))
	pass := os.Getenv("SMTP_PASS")
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	if err := smtp.SendMail(addr, auth, from, []string{toEmail}, message); err != nil {
		return fmt.Errorf("failed to send quotation email: %w", err)
	}
	return nil
}

func (s *Service) CollectQuotationPayment(id string, amount float64) error {
	if amount <= 0 {
		return errors.New("amount must be greater than zero")
	}

	var quote Quotation
	if err := s.db.First(&quote, id).Error; err != nil {
		return errors.New("quotation not found")
	}

	newCollected := quote.CollectedAmount + amount
	newStatus := "Partial"
	if newCollected >= quote.Amount {
		newCollected = quote.Amount
		newStatus = "Completed"
	}

	if err := s.db.Model(&Quotation{}).Where("id = ?", id).Updates(map[string]interface{}{
		"collected_amount": newCollected,
		"status":           newStatus,
	}).Error; err != nil {
		return err
	}

	return s.syncAppointmentFromQuotation(quote.ID)
}
