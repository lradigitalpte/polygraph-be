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
	"strconv"
	"strings"
	"time"

	"my-app/internal/database"
	"my-app/internal/email"
	"my-app/internal/modules/auth"
	"my-app/internal/modules/availability"
	"my-app/internal/modules/subjects"
	"my-app/internal/storage"
	"my-app/internal/timeutil"

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
	if client.EmailDeliveryMode == "" {
		clientType := strings.ToLower(client.ClientType)
		if strings.Contains(clientType, "corporate") || strings.Contains(clientType, "law") {
			client.EmailDeliveryMode = "daily_summary"
			client.EmailExamineeFallback = false
		} else {
			client.EmailDeliveryMode = "immediate"
			client.EmailExamineeFallback = true
		}
		client.EmailBookingNotices = true
		client.EmailSessionReminders = true
		client.EmailSummaryTime = "17:00"
	}
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
	if input.EmailDeliveryMode == "" {
		input.EmailDeliveryMode = client.EmailDeliveryMode
	}
	if input.EmailSummaryTime == "" {
		input.EmailSummaryTime = client.EmailSummaryTime
	}
	validDeliveryMode := map[string]bool{"immediate": true, "daily_summary": true, "important_only": true, "none": true}
	if !validDeliveryMode[input.EmailDeliveryMode] {
		return errors.New("invalid email delivery mode")
	}
	if _, err := time.Parse("15:04", input.EmailSummaryTime); err != nil {
		return errors.New("invalid email summary time")
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
		"email_delivery_mode":      input.EmailDeliveryMode,
		"email_booking_notices":    input.EmailBookingNotices,
		"email_session_reminders":  input.EmailSessionReminders,
		"email_examinee_fallback":  input.EmailExamineeFallback,
		"email_summary_time":       input.EmailSummaryTime,
		"notes":                    strings.TrimSpace(input.Notes),
	}

	return s.db.Model(&client).Updates(updates).Error
}

func (s *Service) CreateAppointment(app *Appointment) error {
	if err := s.validateAppointment(app); err != nil {
		return err
	}
	s.NormalizeNewAppointmentMoney(app)
	if err := s.db.Create(app).Error; err != nil {
		return err
	}
	return s.ensureInvoiceForAppointment(app)
}

func (s *Service) GetAllAppointments(clientID ...string) ([]Appointment, error) {
	var appointments []Appointment
	query := s.db.Preload("Client").Preload("Subject").Order("scheduled_at DESC")
	if len(clientID) > 0 && strings.TrimSpace(clientID[0]) != "" {
		query = query.Where("client_id = ?", strings.TrimSpace(clientID[0]))
	}
	err := query.Find(&appointments).Error
	return appointments, err
}

func (s *Service) GetAppointmentsForExaminer(examinerID uint, clientID ...string) ([]Appointment, error) {
	var appointments []Appointment
	query := s.db.Preload("Client").Preload("Subject").Where("examiner_id = ?", examinerID).Order("scheduled_at DESC")
	if len(clientID) > 0 && strings.TrimSpace(clientID[0]) != "" {
		query = query.Where("client_id = ?", strings.TrimSpace(clientID[0]))
	}
	return appointments, query.Find(&appointments).Error
}

func (s *Service) GetClientDocuments(clientID string) ([]ClientDocument, error) {
	var docs []ClientDocument
	err := s.db.Where("client_id = ?", clientID).Order("created_at DESC").Find(&docs).Error
	if err == nil {
		ctx := context.Background()
		for i := range docs {
			docs[i].URL = storage.SignedURLForStored(ctx, s.storage, docs[i].URL)
		}
	}
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
	if err == nil {
		ctx := context.Background()
		for i := range docs {
			docs[i].URL = storage.SignedURLForStored(ctx, s.storage, docs[i].URL)
		}
	}
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

// ─── Document sharing (one-way: send a stored file to the client) ────────────

// CreateDocumentShare emails the client a tokenised link to view/download an
// already-uploaded document. scope is "client" or "subject".
func (s *Service) CreateDocumentShare(
	scope string,
	clientID uint,
	subjectID *uint,
	documentID uint,
	recipientEmail, recipientName, sentByEmail string,
) (*DocumentShare, error) {
	scope = strings.TrimSpace(scope)

	var (
		name string
		url  string
	)
	switch scope {
	case "subject":
		var doc SubjectDocument
		if err := s.db.First(&doc, documentID).Error; err != nil {
			return nil, errors.New("document not found")
		}
		name, url = doc.Name, doc.URL
		clientID = doc.ClientID
		sid := doc.SubjectID
		subjectID = &sid
	case "client":
		var doc ClientDocument
		if err := s.db.First(&doc, documentID).Error; err != nil {
			return nil, errors.New("document not found")
		}
		name, url = doc.Name, doc.URL
		clientID = doc.ClientID
		subjectID = nil
	default:
		return nil, errors.New("scope must be 'client' or 'subject'")
	}

	if strings.TrimSpace(url) == "" {
		return nil, errors.New("this document has no file to send")
	}

	client, err := s.GetClientByID(strconv.FormatUint(uint64(clientID), 10))
	if err != nil {
		return nil, errors.New("client not found")
	}

	toEmail := strings.TrimSpace(recipientEmail)
	if toEmail == "" {
		toEmail = strings.TrimSpace(client.Email)
	}
	if toEmail == "" || !strings.Contains(toEmail, "@") {
		return nil, errors.New("a valid recipient email is required")
	}

	rname := strings.TrimSpace(recipientName)
	if rname == "" {
		rname = client.Name
	}

	token, err := generateShareToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	share := DocumentShare{
		Token:          token,
		ClientID:       clientID,
		SubjectID:      subjectID,
		Scope:          scope,
		DocumentID:     documentID,
		Name:           name,
		URL:            url,
		RecipientEmail: toEmail,
		RecipientName:  rname,
		Status:         "sent",
		SentAt:         now,
		ExpiresAt:      now.Add(14 * 24 * time.Hour),
		SentByEmail:    strings.TrimSpace(sentByEmail),
	}
	if err := s.db.Create(&share).Error; err != nil {
		return nil, err
	}

	if mailErr := s.sendDocumentShareEmail(&share); mailErr != nil {
		// Keep the share — staff can resend or copy the link manually.
		return &share, fmt.Errorf("document share created but email failed: %w", mailErr)
	}
	return &share, nil
}

func (s *Service) sendDocumentShareEmail(share *DocumentShare) error {
	link := publicShareURL(share.Token)
	subject := fmt.Sprintf("A document has been shared with you — %s", share.Name)
	body := fmt.Sprintf(
		"Hello %s,\n\nPolygraph Forensic System has shared a document with you:\n\n%s\n\nYou can view or download it securely using the link below. The link expires on %s.\n\n%s\n\nThank you,\nPolygraph Forensic System",
		share.RecipientName,
		share.Name,
		share.ExpiresAt.Format("January 2, 2006"),
		link,
	)
	return email.Send(share.RecipientEmail, subject, body)
}

// GetPublicDocumentShare resolves a share by token and marks it viewed on first open.
func (s *Service) GetPublicDocumentShare(token string) (*DocumentShare, error) {
	var share DocumentShare
	if err := s.db.Where("token = ?", token).First(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("shared document not found")
		}
		return nil, err
	}
	if time.Now().UTC().After(share.ExpiresAt) {
		return nil, errors.New("this link has expired")
	}
	if share.Status == "sent" {
		now := time.Now().UTC()
		share.Status = "viewed"
		share.ViewedAt = &now
		_ = s.db.Model(&share).Updates(map[string]interface{}{"status": "viewed", "viewed_at": now}).Error
	}
	// Hand the recipient a short-lived presigned URL rather than the private object URL.
	share.URL = storage.SignedURLForStored(context.Background(), s.storage, share.URL)
	return &share, nil
}

// ListDocumentShares returns shares for a client or a subject, newest first.
func (s *Service) ListDocumentShares(clientID, subjectID string) ([]DocumentShare, error) {
	var shares []DocumentShare
	q := s.db.Order("created_at DESC")
	if trimmed := strings.TrimSpace(subjectID); trimmed != "" {
		q = q.Where("subject_id = ?", trimmed)
	} else if trimmed := strings.TrimSpace(clientID); trimmed != "" {
		q = q.Where("client_id = ?", trimmed)
	}
	return shares, q.Find(&shares).Error
}

// ResendDocumentShare re-emails an existing, non-expired share link.
func (s *Service) ResendDocumentShare(id string) (*DocumentShare, error) {
	var share DocumentShare
	if err := s.db.First(&share, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("document share not found")
		}
		return nil, err
	}
	if time.Now().UTC().After(share.ExpiresAt) {
		return nil, errors.New("this link has expired; send the document again")
	}
	if err := s.sendDocumentShareEmail(&share); err != nil {
		return nil, err
	}
	return &share, nil
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
		"scheduled_at": true, "duration": true, "examiner_id": true, "exam_fee": true,
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
		if !s.sundayBookingsEnabled() && parsed.In(timeutil.ClinicLocation()).Weekday() == time.Sunday {
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
	if _, ok := safe["exam_fee"]; ok {
		org := orgCurrencyCode(s.loadMoneyRates())
		if strings.TrimSpace(appointment.FeeCurrency) == "" {
			_ = s.db.Model(&appointment).Update("fee_currency", org).Error
			appointment.FeeCurrency = org
		}
		typePrices := s.loadExamTypePriceIndex()
		rates := s.loadMoneyRates()
		s.normalizeAppointmentMoney(&appointment, typePrices, rates)
		_ = s.syncQuotationFromAppointment(appointment.ID)
		if err := s.db.Preload("Client").First(&appointment, id).Error; err != nil {
			return nil, err
		}
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

// EmailAppointmentConfirmation emails the examinee that their session has been
// scheduled, with the date/time and examiner. Best-effort: returns nil (no-op)
// when there is no usable recipient email, so booking flows can ignore the result.
func (s *Service) EmailAppointmentConfirmation(apptID uint) error {
	var appt Appointment
	if err := s.db.Preload("Client").First(&appt, apptID).Error; err != nil {
		return err
	}
	if !appt.Client.EmailBookingNotices || appt.Client.EmailDeliveryMode == "none" || appt.Client.EmailDeliveryMode == "important_only" {
		return nil
	}

	// Prefer the examinee's own email; fall back to the client/organisation email.
	var subj subjects.Subject
	_ = s.db.First(&subj, appt.SubjectID).Error
	recipientName := strings.TrimSpace(subj.FirstName + " " + subj.LastName)
	toEmail := strings.TrimSpace(subj.Email)
	if toEmail == "" {
		if appt.Client.EmailDeliveryMode != "immediate" || !appt.Client.EmailExamineeFallback {
			return nil
		}
		toEmail = strings.TrimSpace(appt.Client.Email)
		if recipientName == "" {
			recipientName = appt.Client.Name
		}
	}
	if toEmail == "" || !strings.Contains(toEmail, "@") {
		return nil // no one to notify — skip
	}
	if recipientName == "" {
		recipientName = "there"
	}

	var examiner auth.User
	examinerName := "your examiner"
	if err := s.db.First(&examiner, appt.ExaminerID).Error; err == nil && strings.TrimSpace(examiner.Name) != "" {
		examinerName = examiner.Name
	}

	when := appt.ScheduledAt.Format("Monday, 2 January 2006 at 15:04")
	subject := fmt.Sprintf("Your polygraph session is scheduled — %s", formatAppointmentCode(appt.ID))
	body := fmt.Sprintf(
		"Hello %s,\n\nYour polygraph examination has been scheduled. Here are the details:\n\nReference: %s\nDate & time: %s\nDuration: %d minutes\nExaminer: %s\n\nPlease arrive 15 minutes early and bring valid photo ID. If you need to reschedule, reply to this email or contact our office.\n\nThank you,\nPolygraph Forensic System",
		recipientName,
		formatAppointmentCode(appt.ID),
		when,
		appt.Duration,
		examinerName,
	)

	return sendSMTPMail(toEmail, subject, body)
}

// EmailAppointmentReminder emails the examinee a pre-session reminder.
// Best-effort: returns nil (no-op) when there is no usable recipient email.
func (s *Service) EmailAppointmentReminder(apptID uint) error {
	var appt Appointment
	if err := s.db.Preload("Client").First(&appt, apptID).Error; err != nil {
		return err
	}
	if !appt.Client.EmailSessionReminders || appt.Client.EmailDeliveryMode == "none" || appt.Client.EmailDeliveryMode == "important_only" {
		return nil
	}

	var subj subjects.Subject
	_ = s.db.First(&subj, appt.SubjectID).Error
	recipientName := strings.TrimSpace(subj.FirstName + " " + subj.LastName)
	toEmail := strings.TrimSpace(subj.Email)
	if toEmail == "" {
		if appt.Client.EmailDeliveryMode != "immediate" || !appt.Client.EmailExamineeFallback {
			return nil
		}
		toEmail = strings.TrimSpace(appt.Client.Email)
		if recipientName == "" {
			recipientName = appt.Client.Name
		}
	}
	if toEmail == "" || !strings.Contains(toEmail, "@") {
		return nil
	}
	if recipientName == "" {
		recipientName = "there"
	}

	var examiner auth.User
	examinerName := "your examiner"
	if err := s.db.First(&examiner, appt.ExaminerID).Error; err == nil && strings.TrimSpace(examiner.Name) != "" {
		examinerName = examiner.Name
	}

	when := appt.ScheduledAt.Format("Monday, 2 January 2006 at 15:04")
	subject := fmt.Sprintf("Reminder: your polygraph session — %s", formatAppointmentCode(appt.ID))
	body := fmt.Sprintf(
		"Hello %s,\n\nThis is a reminder of your upcoming polygraph examination.\n\nReference: %s\nDate & time: %s\nDuration: %d minutes\nExaminer: %s\n\nPlease arrive 15 minutes early, bring valid photo ID, and avoid caffeine for 4 hours before the session. Reply to this email if you need to reschedule.\n\nThank you,\nPolygraph Forensic System",
		recipientName,
		formatAppointmentCode(appt.ID),
		when,
		appt.Duration,
		examinerName,
	)

	return sendSMTPMail(toEmail, subject, body)
}

// RunDueReminders sends a one-time reminder for every non-cancelled, non-completed
// appointment scheduled within the next withinHours that has not been reminded yet.
// It is safe to call repeatedly (e.g. hourly from cron): RemindedAt dedupes sends.
// Returns the number of reminders successfully sent.
func (s *Service) RunDueReminders(withinHours int) (int, error) {
	if withinHours <= 0 {
		withinHours = 24
	}
	now := time.Now().UTC()
	cutoff := now.Add(time.Duration(withinHours) * time.Hour)

	var due []Appointment
	err := s.db.
		Where("reminded_at IS NULL").
		Where("status NOT IN ?", []string{"cancelled", "completed"}).
		Where("scheduled_at > ? AND scheduled_at <= ?", now, cutoff).
		Find(&due).Error
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, appt := range due {
		if mailErr := s.EmailAppointmentReminder(appt.ID); mailErr != nil {
			// Skip marking on failure so the next run retries this one.
			continue
		}
		stamp := time.Now().UTC()
		if updErr := s.db.Model(&Appointment{}).Where("id = ?", appt.ID).
			Update("reminded_at", &stamp).Error; updErr != nil {
			continue
		}
		sent++
	}
	return sent, nil
}

// RunCorporateDailySummaries sends one consolidated operational email per opted-in
// corporate client. It replaces per-examinee fallback messages when examinees do
// not have their own email address.
func (s *Service) RunCorporateDailySummaries() (int, error) {
	var clients []Client
	if err := s.db.Where("email_delivery_mode = ?", "daily_summary").Where("email <> ''").Find(&clients).Error; err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	dubai, _ := time.LoadLocation("Asia/Dubai")
	if dubai == nil {
		dubai = time.FixedZone("Dubai", 4*60*60)
	}
	localNow := now.In(dubai)
	sent := 0
	for _, client := range clients {
		summaryTime := strings.TrimSpace(client.EmailSummaryTime)
		if summaryTime == "" {
			summaryTime = "17:00"
		}
		parts := strings.Split(summaryTime, ":")
		if len(parts) != 2 {
			continue
		}
		hour, hourErr := strconv.Atoi(parts[0])
		minute, minuteErr := strconv.Atoi(parts[1])
		if hourErr != nil || minuteErr != nil || localNow.Hour() != hour || localNow.Minute() < minute || localNow.Minute() >= minute+60 {
			continue
		}
		if client.LastEmailSummaryAt != nil {
			last := client.LastEmailSummaryAt.In(dubai)
			if last.Year() == localNow.Year() && last.YearDay() == localNow.YearDay() {
				continue
			}
		}
		since := now.Add(-24 * time.Hour)
		if client.LastEmailSummaryAt != nil && client.LastEmailSummaryAt.After(since) {
			since = *client.LastEmailSummaryAt
		}
		var appointments []Appointment
		if err := s.db.Preload("Subject").Where("client_id = ?", client.ID).
			Where("created_at >= ? OR updated_at >= ? OR (scheduled_at >= ? AND scheduled_at <= ?)", since, since, now, now.Add(7*24*time.Hour)).
			Order("scheduled_at ASC").Find(&appointments).Error; err != nil {
			continue
		}
		if len(appointments) == 0 {
			_ = s.db.Model(&Client{}).Where("id = ?", client.ID).Update("last_email_summary_at", now).Error
			continue
		}
		newBookings, completed, cancelled := 0, 0, 0
		lines := make([]string, 0, len(appointments))
		for _, appt := range appointments {
			if !appt.CreatedAt.Before(since) {
				newBookings++
			}
			switch strings.ToLower(appt.Status) {
			case "completed":
				completed++
			case "cancelled", "canceled":
				cancelled++
			}
			name := strings.TrimSpace(appt.Subject.FirstName + " " + appt.Subject.LastName)
			if name == "" {
				name = fmt.Sprintf("Examinee #%d", appt.SubjectID)
			}
			lines = append(lines, fmt.Sprintf("• %s — %s — %s", name, appt.ScheduledAt.Format("Mon, 2 Jan 2006 at 15:04"), strings.Title(appt.Status)))
		}
		body := fmt.Sprintf("Hello %s,\n\nHere is your consolidated Polygraph activity summary.\n\nNew bookings: %d\nCompleted: %d\nCancelled: %d\n\nSessions\n%s\n\nInvoices and reports are sent separately only when requested.\n\nThank you,\nPolygraph Forensic System", client.Name, newBookings, completed, cancelled, strings.Join(lines, "\n"))
		if err := sendSMTPMail(client.Email, "Daily Polygraph activity summary", body); err != nil {
			continue
		}
		if err := s.db.Model(&Client{}).Where("id = ?", client.ID).Update("last_email_summary_at", now).Error; err != nil {
			continue
		}
		sent++
	}
	return sent, nil
}

// EmailInvoiceForAppointment emails the invoice/quotation generated for an
// appointment to the client. Best-effort: returns nil (no-op) when there is no
// fee or no valid client email, so callers can ignore the result on booking.
func (s *Service) EmailInvoiceForAppointment(apptID uint) error {
	var appt Appointment
	if err := s.db.Preload("Client").First(&appt, apptID).Error; err != nil {
		return err
	}

	toEmail := strings.TrimSpace(appt.Client.Email)
	if toEmail == "" || !strings.Contains(toEmail, "@") {
		return nil // nothing to send to — skip silently
	}
	if appt.ExamFee <= 0 {
		return nil // no charge, no invoice to send
	}

	// Find the invoice generated for this appointment.
	var quote Quotation
	if err := s.db.Where("appointment_id = ?", apptID).First(&quote).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // no invoice was created (e.g. zero fee) — skip
		}
		return err
	}

	balance := appt.ExamFee - appt.CollectedAmount
	if balance < 0 {
		balance = 0
	}

	subject := fmt.Sprintf("Invoice %s — Polygraph Forensic System", quote.Code)
	body := fmt.Sprintf(
		"Hello %s,\n\nThank you for booking your polygraph session (%s). Please find your invoice details below.\n\nInvoice: %s\nDescription: %s\nTotal: $%.2f\nPaid to date: $%.2f\nBalance due: $%.2f\nPayment method: %s\n\nPlease arrange payment ahead of your appointment. Reply to this email if you have any questions.\n\nThank you,\nPolygraph Forensic System",
		appt.Client.Name,
		formatAppointmentCode(appt.ID),
		quote.Code,
		quote.Title,
		appt.ExamFee,
		appt.CollectedAmount,
		balance,
		appt.PaymentMode,
	)

	if err := sendSMTPMail(toEmail, subject, body); err != nil {
		return err
	}

	now := time.Now().UTC()
	_ = s.db.Model(&Quotation{}).Where("id = ?", quote.ID).Updates(map[string]interface{}{
		"sent_to_email": toEmail,
		"email_subject": subject,
		"email_body":    body,
		"sent_at":       now,
		"status":        "Sent",
	}).Error
	return nil
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
	// Past scheduled_at is allowed so staff can log exams that already happened
	// (forgot to book, report generation, etc.). The booking UI surfaces a backdated notice.
	if !s.sundayBookingsEnabled() && app.ScheduledAt.In(timeutil.ClinicLocation()).Weekday() == time.Sunday {
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

func (s *Service) sundayBookingsEnabled() bool {
	var config struct {
		SundayBookingsEnabled bool `gorm:"column:sunday_bookings_enabled"`
	}
	if err := s.db.Table("organization_settings").Select("sunday_bookings_enabled").Where("id = ?", 1).Take(&config).Error; err != nil {
		return false
	}
	return config.SundayBookingsEnabled
}

func (s *Service) hasAvailabilityConflict(examinerID uint, scheduledAt time.Time, duration int) (bool, error) {
	local := scheduledAt.In(timeutil.ClinicLocation())
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, timeutil.ClinicLocation())
	dayEnd := dayStart.Add(24 * time.Hour)

	var blocks []availability.Block
	if err := s.db.Where("examiner_id = ? AND date >= ? AND date < ?", examinerID, dayStart, dayEnd).Find(&blocks).Error; err != nil {
		return false, err
	}
	if len(blocks) == 0 {
		return false, nil
	}

	startMinutes := timeutil.ClockMinutes(scheduledAt)
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
	input.Title = truncate(input.Title, 255)
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

	if input.Currency == "" {
		defaultCurrency := "USD"
		var org struct {
			Currency string `gorm:"column:currency"`
		}
		if err := s.db.Table("organization_settings").Select("currency").Where("id = ?", 1).First(&org).Error; err == nil && org.Currency != "" {
			defaultCurrency = org.Currency
		}
		input.Currency = defaultCurrency
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
	return email.Send(toEmail, subject, body)
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

type ConvertQuotationInput struct {
	SubjectID   uint      `json:"subject_id"`
	ExaminerID  uint      `json:"examiner_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Duration    int       `json:"duration"`
}

// ConvertQuotationToAppointment turns a standalone quotation into a booked
// appointment and links the existing quotation to it (so it becomes that booking's
// invoice — no duplicate is created). The quoted amount carries over as the fee.
func (s *Service) ConvertQuotationToAppointment(quotationID string, in ConvertQuotationInput) (*Appointment, error) {
	var quote Quotation
	if err := s.db.First(&quote, quotationID).Error; err != nil {
		return nil, errors.New("quotation not found")
	}
	if quote.AppointmentID != nil {
		return nil, errors.New("this quotation is already linked to a booking")
	}
	if in.Duration <= 0 {
		in.Duration = 150
	}

	notes := quote.Title
	if strings.TrimSpace(quote.Description) != "" {
		notes = quote.Title + "\n\n" + quote.Description
	}

	appt := Appointment{
		ClientID:        quote.ClientID,
		SubjectID:       in.SubjectID,
		ExaminerID:      in.ExaminerID,
		ScheduledAt:     in.ScheduledAt,
		Duration:        in.Duration,
		ExamFee:         quote.Amount,
		FeeCurrency:     strings.ToUpper(strings.TrimSpace(quote.Currency)),
		CollectedAmount: quote.CollectedAmount,
		Status:          "confirmed",
		PaymentStatus:   quoteStatusToPaymentStatus(quote.Status, quote.CollectedAmount, quote.Amount),
		Notes:           notes,
	}
	if strings.TrimSpace(appt.FeeCurrency) == "" {
		appt.FeeCurrency = orgCurrencyCode(s.loadMoneyRates())
	}
	// validateAppointment runs the Sunday/availability/overlap checks and sets defaults.
	if err := s.validateAppointment(&appt); err != nil {
		return nil, err
	}
	typePrices := s.loadExamTypePriceIndex()
	rates := s.loadMoneyRates()
	s.normalizeAppointmentMoney(&appt, typePrices, rates)
	// Create directly (not via CreateAppointment) so we don't auto-generate a second
	// invoice — we link the existing quotation below instead.
	if err := s.db.Create(&appt).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&Quotation{}).Where("id = ?", quote.ID).Update("appointment_id", appt.ID).Error; err != nil {
		return nil, err
	}
	return &appt, nil
}

// DeleteQuotation removes a quotation/invoice. The linked appointment (if any) is
// left intact — only the billing record is removed.
func (s *Service) DeleteQuotation(id string) error {
	result := s.db.Delete(&Quotation{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("quotation not found")
	}
	return nil
}

func (s *Service) BulkEditPrices(targets []struct {
	Source        string
	ID            uint
	AppointmentID *uint
	QuotationID   *uint
}, newPrice float64) error {
	org := orgCurrencyCode(s.loadMoneyRates())
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, t := range targets {
			if t.AppointmentID != nil && *t.AppointmentID > 0 {
				var appt Appointment
				if err := tx.First(&appt, *t.AppointmentID).Error; err == nil {
					updates := map[string]interface{}{
						"exam_fee":     newPrice,
						"fee_currency": org,
					}
					if appt.PaymentStatus == "Paid" || appt.CollectedAmount > 0 {
						updates["collected_amount"] = newPrice
					}
					if err := tx.Model(&appt).Updates(updates).Error; err != nil {
						return err
					}
				}
			} else if t.QuotationID != nil && *t.QuotationID > 0 {
				var quote Quotation
				if err := tx.First(&quote, *t.QuotationID).Error; err == nil {
					updates := map[string]interface{}{
						"amount":   newPrice,
						"currency": org,
					}
					if quote.Status == "Paid" || quote.CollectedAmount > 0 {
						updates["collected_amount"] = newPrice
					}
					if err := tx.Model(&quote).Updates(updates).Error; err != nil {
						return err
					}
				}
			} else if t.Source == "booking" || t.Source == "session" {
				var appt Appointment
				if err := tx.First(&appt, t.ID).Error; err == nil {
					updates := map[string]interface{}{
						"exam_fee":     newPrice,
						"fee_currency": org,
					}
					if appt.PaymentStatus == "Paid" || appt.CollectedAmount > 0 {
						updates["collected_amount"] = newPrice
					}
					if err := tx.Model(&appt).Updates(updates).Error; err != nil {
						return err
					}
				}
			} else if t.Source == "quote" {
				var quote Quotation
				if err := tx.First(&quote, t.ID).Error; err == nil {
					updates := map[string]interface{}{
						"amount":   newPrice,
						"currency": org,
					}
					if quote.Status == "Paid" || quote.CollectedAmount > 0 {
						updates["collected_amount"] = newPrice
					}
					if err := tx.Model(&quote).Updates(updates).Error; err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}
