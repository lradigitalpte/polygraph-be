package forms

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"my-app/internal/database"
	"my-app/internal/modules/appointments"
	"my-app/internal/modules/subjects"

	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

func (s *Service) ListTemplates(audience string) ([]FormTemplate, error) {
	var templates []FormTemplate
	q := s.db.Where("active = ?", true).Order("category ASC, name ASC")
	if trimmed := strings.TrimSpace(audience); trimmed != "" && trimmed != "all" {
		q = q.Where("audience IN ?", []string{"all", trimmed})
	}
	err := q.Find(&templates).Error
	return templates, err
}

func (s *Service) ListPendingRequests(limit int) ([]FormRequest, error) {
	if limit <= 0 {
		limit = 50
	}
	var requests []FormRequest
	err := s.db.Preload("Template").
		Where("status IN ? AND expires_at > ?", []string{"sent", "opened"}, time.Now().UTC()).
		Order("sent_at ASC").
		Limit(limit).
		Find(&requests).Error
	return requests, err
}

func (s *Service) ListClientRequests(clientID string) ([]FormRequest, error) {
	var requests []FormRequest
	err := s.db.Preload("Template").
		Where("client_id = ?", clientID).
		Order("created_at DESC").
		Find(&requests).Error
	return requests, err
}

func (s *Service) ListSubjectRequests(subjectID string) ([]FormRequest, error) {
	var requests []FormRequest
	err := s.db.Preload("Template").
		Where("subject_id = ?", subjectID).
		Order("created_at DESC").
		Find(&requests).Error
	return requests, err
}

type SendFormInput struct {
	TemplateID     uint
	ClientID       uint
	SubjectID      *uint
	RecipientEmail string
	RecipientName  string
	SentByEmail    string
}

func (s *Service) SendFormRequest(input SendFormInput) (*FormRequest, error) {
	var template FormTemplate
	if err := s.db.First(&template, input.TemplateID).Error; err != nil {
		return nil, errors.New("form template not found")
	}
	if !template.Active {
		return nil, errors.New("form template is not active")
	}

	var client appointments.Client
	if err := s.db.First(&client, input.ClientID).Error; err != nil {
		return nil, errors.New("client not found")
	}

	toEmail := strings.TrimSpace(input.RecipientEmail)
	if toEmail == "" {
		toEmail = strings.TrimSpace(client.Email)
	}
	if toEmail == "" || !strings.Contains(toEmail, "@") {
		return nil, errors.New("valid recipient email is required")
	}

	recipientName := strings.TrimSpace(input.RecipientName)
	if recipientName == "" {
		recipientName = client.Name
	}

	if input.SubjectID != nil && *input.SubjectID > 0 {
		var subj subjects.Subject
		if err := s.db.First(&subj, *input.SubjectID).Error; err != nil {
			return nil, errors.New("examinee not found")
		}
		if subj.ClientID != nil && *subj.ClientID != input.ClientID {
			return nil, errors.New("examinee does not belong to this client account")
		}
	}

	token, err := generateToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	req := FormRequest{
		Token:          token,
		TemplateID:     template.ID,
		ClientID:       input.ClientID,
		SubjectID:      input.SubjectID,
		RecipientEmail: toEmail,
		RecipientName:  recipientName,
		Status:         "sent",
		SentAt:         now,
		ExpiresAt:      now.Add(14 * 24 * time.Hour),
		SentByEmail:    strings.TrimSpace(input.SentByEmail),
	}

	if err := s.db.Create(&req).Error; err != nil {
		return nil, err
	}

	link := publicFormURL(token)
	subject := fmt.Sprintf("Please complete: %s — Polygraph Forensic System", template.Name)
	body := fmt.Sprintf(
		"Hello %s,\n\nPlease complete the following form before your session:\n\n%s\n\n%s\n\nThis link expires on %s.\n\nThank you,\nPolygraph Forensic System",
		recipientName,
		template.Name,
		link,
		req.ExpiresAt.Format("January 2, 2006"),
	)
	if err := sendSMTPMail(toEmail, subject, body); err != nil {
		return &req, fmt.Errorf("form request created but email failed: %w", err)
	}

	if err := s.db.Preload("Template").First(&req, req.ID).Error; err != nil {
		return &req, nil
	}
	return &req, nil
}

func (s *Service) ResendFormRequest(id string) (*FormRequest, error) {
	var req FormRequest
	if err := s.db.Preload("Template").First(&req, id).Error; err != nil {
		return nil, errors.New("form request not found")
	}
	if req.Status == "completed" {
		return nil, errors.New("form already completed")
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		return nil, errors.New("form link has expired; send a new request")
	}

	link := publicFormURL(req.Token)
	subject := fmt.Sprintf("Reminder: %s — Polygraph Forensic System", req.Template.Name)
	body := fmt.Sprintf(
		"Hello %s,\n\nThis is a reminder to complete your form:\n\n%s\n\n%s\n\nThank you,\nPolygraph Forensic System",
		req.RecipientName,
		req.Template.Name,
		link,
	)
	if err := sendSMTPMail(req.RecipientEmail, subject, body); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	req.SentAt = now
	req.Status = "sent"
	_ = s.db.Model(&req).Updates(map[string]interface{}{"sent_at": now, "status": "sent"}).Error
	return &req, nil
}

type PublicFormView struct {
	Request  FormRequest `json:"request"`
	Template FormTemplate `json:"template"`
	Schema   FormSchema  `json:"schema"`
}

func (s *Service) GetPublicForm(token string) (*PublicFormView, error) {
	var req FormRequest
	if err := s.db.Preload("Template").Where("token = ?", token).First(&req).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("form link not found")
		}
		return nil, err
	}

	if req.Status == "completed" {
		return nil, errors.New("this form has already been submitted")
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		_ = s.db.Model(&req).Update("status", "expired").Error
		return nil, errors.New("this form link has expired")
	}

	schema, err := ParseSchema(req.Template.SchemaJSON)
	if err != nil {
		return nil, errors.New("invalid form template")
	}

	if req.Status == "sent" {
		now := time.Now().UTC()
		req.Status = "opened"
		req.OpenedAt = &now
		_ = s.db.Model(&req).Updates(map[string]interface{}{"status": "opened", "opened_at": now}).Error
	}

	return &PublicFormView{Request: req, Template: req.Template, Schema: schema}, nil
}

func (s *Service) SubmitPublicForm(token string, data map[string]interface{}) (*FormRequest, error) {
	var req FormRequest
	if err := s.db.Preload("Template").Where("token = ?", token).First(&req).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("form link not found")
		}
		return nil, err
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		return nil, errors.New("this form link has expired")
	}
	if req.Status == "completed" {
		return nil, errors.New("this form has already been submitted")
	}

	schema, err := ParseSchema(req.Template.SchemaJSON)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateSubmission(data); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	docType := req.Template.Category
	if docType == "" {
		docType = "other"
	}
	if docType == "intake" {
		docType = "intake_form"
	} else if docType == "consent" {
		docType = "consent_form"
	}

	clientDoc := appointments.ClientDocument{
		ClientID: req.ClientID,
		Name:     req.Template.Name + " (online)",
		Type:     docType,
		Source:   "online_form",
		FormData: string(payload),
	}
	if err := s.db.Create(&clientDoc).Error; err != nil {
		return nil, err
	}

	var subjectDocID *uint
	if req.SubjectID != nil && *req.SubjectID > 0 {
		subDoc := appointments.SubjectDocument{
			SubjectID: *req.SubjectID,
			ClientID:  req.ClientID,
			Name:      req.Template.Name + " (online)",
			Type:      docType,
			Source:    "online_form",
			FormData:  string(payload),
		}
		if err := s.db.Create(&subDoc).Error; err == nil {
			subjectDocID = &subDoc.ID
		}
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"status":              "completed",
		"completed_at":        now,
		"submitted_data":      string(payload),
		"client_document_id":  clientDoc.ID,
		"subject_document_id": subjectDocID,
	}
	if err := s.db.Model(&req).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.Preload("Template").First(&req, req.ID).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func generateToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) GetTemplateByID(id string) (*FormTemplate, error) {
	var tpl FormTemplate
	if err := s.db.First(&tpl, id).Error; err != nil {
		return nil, errors.New("template not found")
	}
	return &tpl, nil
}

func ParseClientIDParam(id string) (uint, error) {
	v, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return 0, errors.New("invalid id")
	}
	return uint(v), nil
}
