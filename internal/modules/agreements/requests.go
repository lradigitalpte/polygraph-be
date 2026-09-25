package agreements

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"my-app/internal/modules/appointments"
	"my-app/internal/modules/exams"
	"my-app/internal/modules/settings"
)

const linkValidity = 14 * 24 * time.Hour

var (
	ErrRequestNotFound = errors.New("agreement request not found")
	// ErrLinkUnavailable covers voided and expired links; the message is shown to the client.
	ErrLinkUnavailable = errors.New("link unavailable")
)

type linkUnavailableError struct{ msg string }

func (e linkUnavailableError) Error() string { return e.msg }
func (e linkUnavailableError) Is(target error) bool {
	return target == ErrLinkUnavailable
}

// isOpen reports whether the client can still act on the request.
func (r *AgreementRequest) isOpen() bool {
	return r.Status == StatusSent || r.Status == StatusViewed
}

// refreshExpiry marks an open request as expired once its link has lapsed.
func (s *Service) refreshExpiry(r *AgreementRequest) {
	if r.isOpen() && time.Now().UTC().After(r.ExpiresAt) {
		r.Status = StatusExpired
		s.db.Model(&AgreementRequest{}).
			Where("id = ? AND status IN ?", r.ID, []string{StatusSent, StatusViewed}).
			Update("status", StatusExpired)
	}
}

func (s *Service) organization() settings.OrganizationSettings {
	var org settings.OrganizationSettings
	if err := s.db.First(&org, 1).Error; err != nil || strings.TrimSpace(org.Name) == "" {
		org.Name = "Polygraph Forensic Labs"
	}
	return org
}

// --- Staff side -------------------------------------------------------------------

type SendInput struct {
	AppointmentID  *uint  `json:"appointment_id"`
	TemplateIDs    []uint `json:"template_ids"`
	RecipientEmail string `json:"recipient_email"`
	RecipientName  string `json:"recipient_name"`
}

// SendResult carries the created request plus a non-fatal email error, so staff can
// still copy the link when delivery fails.
type SendResult struct {
	Request    *AgreementRequest `json:"request"`
	Link       string            `json:"link"`
	EmailError string            `json:"email_error,omitempty"`
}

func (s *Service) SendRequest(clientID uint, input SendInput, sentBy string) (*SendResult, error) {
	var client appointments.Client
	if err := s.db.First(&client, clientID).Error; err != nil {
		return nil, errors.New("client not found")
	}
	if isOrganizationClientType(client.ClientType) {
		return nil, errors.New("agreements are only sent to individual clients")
	}

	if input.AppointmentID != nil && *input.AppointmentID > 0 {
		var appt appointments.Appointment
		if err := s.db.First(&appt, *input.AppointmentID).Error; err != nil {
			return nil, errors.New("booking not found")
		}
		if appt.ClientID != clientID {
			return nil, errors.New("booking does not belong to this client")
		}
		if strings.EqualFold(appt.Status, "cancelled") {
			return nil, errors.New("booking is cancelled")
		}
		var open int64
		s.db.Model(&AgreementRequest{}).
			Where("appointment_id = ? AND status IN ? AND expires_at > ?",
				appt.ID, []string{StatusSent, StatusViewed}, time.Now().UTC()).
			Count(&open)
		if open > 0 {
			return nil, errors.New("this booking already has agreements awaiting signature — resend or void them first")
		}
	} else {
		input.AppointmentID = nil
	}

	ids := uniqueIDs(input.TemplateIDs)
	if len(ids) == 0 {
		return nil, errors.New("select at least one agreement")
	}
	var templates []AgreementTemplate
	if err := s.db.Where("id IN ? AND active = ?", ids, true).
		Order("sort_order ASC, id ASC").Find(&templates).Error; err != nil {
		return nil, err
	}
	if len(templates) != len(ids) {
		return nil, errors.New("one or more selected agreements are inactive or were deleted")
	}

	toEmail := strings.TrimSpace(input.RecipientEmail)
	if toEmail == "" {
		toEmail = strings.TrimSpace(client.Email)
	}
	if toEmail == "" || !strings.Contains(toEmail, "@") || strings.ContainsAny(toEmail, " \r\n") {
		return nil, errors.New("a valid recipient email is required")
	}
	recipientName := strings.TrimSpace(input.RecipientName)
	if recipientName == "" {
		recipientName = client.Name
	}

	token, err := generateToken()
	if err != nil {
		return nil, err
	}
	items := make([]AgreementRequestItem, 0, len(templates))
	for i, t := range templates {
		items = append(items, AgreementRequestItem{
			TemplateID:      t.ID,
			TemplateVersion: t.Version,
			Kind:            t.Kind,
			Title:           t.Title,
			BodyHTML:        t.BodyHTML,
			SortOrder:       i + 1,
		})
	}
	now := time.Now().UTC()
	req := AgreementRequest{
		Token:          token,
		ClientID:       clientID,
		AppointmentID:  input.AppointmentID,
		RecipientEmail: toEmail,
		RecipientName:  recipientName,
		Status:         StatusSent,
		SentAt:         now,
		ExpiresAt:      now.Add(linkValidity),
		SentByEmail:    strings.TrimSpace(sentBy),
		ContentHash:    contentHash(items),
		Items:          items,
	}
	if err := s.db.Create(&req).Error; err != nil {
		return nil, err
	}

	result := &SendResult{Request: &req, Link: publicAgreementURL(token)}
	if err := sendRequestEmail(&req, s.organization().Name, false); err != nil {
		result.EmailError = "Agreements were created but the email could not be sent. Copy the link and share it with the client."
	}
	return result, nil
}

func uniqueIDs(ids []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// RequestSummary is the staff-facing row: the request, its agreement titles and the link.
type RequestSummary struct {
	AgreementRequest
	Link string `json:"link,omitempty"`
}

func (s *Service) toSummary(r AgreementRequest) RequestSummary {
	sum := RequestSummary{AgreementRequest: r}
	if r.isOpen() {
		sum.Link = publicAgreementURL(r.Token)
	}
	return sum
}

func (s *Service) ListClientRequests(clientID uint) ([]RequestSummary, error) {
	var rows []AgreementRequest
	err := s.db.Omit("pdf_data", "signature_data_url").Where("client_id = ?", clientID).
		Preload("Items", func(db *gorm.DB) *gorm.DB {
			// Titles only; the full terms are loaded on the detail view.
			return db.Select("id", "request_id", "template_id", "template_version", "kind", "title", "sort_order", "accepted_at").
				Order("sort_order ASC")
		}).
		Order("created_at DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]RequestSummary, 0, len(rows))
	for i := range rows {
		s.refreshExpiry(&rows[i])
		out = append(out, s.toSummary(rows[i]))
	}
	return out, nil
}

// RequestDetail adds the signature image, which list views leave out.
type RequestDetail struct {
	RequestSummary
	SignatureDataURL string `json:"signature_data_url,omitempty"`
}

func (s *Service) loadRequest(id string) (*AgreementRequest, error) {
	parsed, err := strconv.ParseUint(id, 10, 64)
	if err != nil || parsed == 0 {
		return nil, ErrRequestNotFound
	}
	var req AgreementRequest
	err = s.db.Omit("pdf_data").Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order ASC") }).
		First(&req, uint(parsed)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	s.refreshExpiry(&req)
	return &req, nil
}

func (s *Service) GetRequest(id string) (*RequestDetail, error) {
	req, err := s.loadRequest(id)
	if err != nil {
		return nil, err
	}
	return &RequestDetail{RequestSummary: s.toSummary(*req), SignatureDataURL: req.SignatureDataURL}, nil
}

func (s *Service) ResendRequest(id string) (*RequestSummary, error) {
	req, err := s.loadRequest(id)
	if err != nil {
		return nil, err
	}
	if !req.isOpen() {
		return nil, errors.New("only agreements awaiting signature can be resent")
	}
	if err := sendRequestEmail(req, s.organization().Name, true); err != nil {
		return nil, errors.New("the reminder email could not be sent")
	}
	sum := s.toSummary(*req)
	return &sum, nil
}

func (s *Service) VoidRequest(id string, actor string) (*RequestSummary, error) {
	req, err := s.loadRequest(id)
	if err != nil {
		return nil, err
	}
	if req.Status == StatusSigned {
		return nil, errors.New("signed agreements cannot be voided")
	}
	if req.Status == StatusVoided {
		return nil, errors.New("agreement is already voided")
	}
	now := time.Now().UTC()
	if err := s.db.Model(req).Updates(map[string]interface{}{
		"status":    StatusVoided,
		"voided_at": now,
		"voided_by": actor,
	}).Error; err != nil {
		return nil, err
	}
	req.Status, req.VoidedAt, req.VoidedBy = StatusVoided, &now, actor
	sum := s.toSummary(*req)
	return &sum, nil
}

// --- Public (client) side ---------------------------------------------------------

type PublicOrganization struct {
	Name         string `json:"name"`
	Website      string `json:"website,omitempty"`
	LogoDataURL  string `json:"logo_data_url,omitempty"`
	Phone        string `json:"phone,omitempty"`
	SupportEmail string `json:"support_email,omitempty"`
	Address      string `json:"address,omitempty"`
}

type PublicBooking struct {
	ScheduledAt time.Time `json:"scheduled_at"`
	Duration    int       `json:"duration"`
	ExamType    string    `json:"exam_type,omitempty"`
	ExamFee     float64   `json:"exam_fee"`
	Collected   float64   `json:"collected_amount"`
	Currency    string    `json:"currency"`
}

type PublicItem struct {
	ID         uint       `json:"id"`
	Title      string     `json:"title"`
	Kind       string     `json:"kind"`
	BodyHTML   string     `json:"body_html"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
}

type PublicView struct {
	Status        string             `json:"status"`
	RecipientName string             `json:"recipient_name"`
	ExpiresAt     time.Time          `json:"expires_at"`
	SignedAt      *time.Time         `json:"signed_at,omitempty"`
	SignedName    string             `json:"signed_name,omitempty"`
	DeclinedAt    *time.Time         `json:"declined_at,omitempty"`
	Organization  PublicOrganization `json:"organization"`
	Booking       *PublicBooking     `json:"booking,omitempty"`
	Items         []PublicItem       `json:"items"`
}

func (s *Service) loadByToken(token string) (*AgreementRequest, error) {
	token = strings.TrimSpace(token)
	if len(token) != 64 {
		return nil, ErrRequestNotFound
	}
	var req AgreementRequest
	err := s.db.Omit("pdf_data").Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("sort_order ASC") }).
		Where("token = ?", token).First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	s.refreshExpiry(&req)
	switch req.Status {
	case StatusVoided:
		return nil, linkUnavailableError{"This agreement link has been withdrawn. Please contact us if you need a new one."}
	case StatusExpired:
		return nil, linkUnavailableError{"This agreement link has expired. Please contact us to receive a new one."}
	}
	return &req, nil
}

func (s *Service) GetPublicView(token string) (*PublicView, error) {
	req, err := s.loadByToken(token)
	if err != nil {
		return nil, err
	}
	if req.Status == StatusSent {
		now := time.Now().UTC()
		if s.db.Model(&AgreementRequest{}).
			Where("id = ? AND status = ?", req.ID, StatusSent).
			Updates(map[string]interface{}{"status": StatusViewed, "viewed_at": now}).RowsAffected == 1 {
			req.Status, req.ViewedAt = StatusViewed, &now
		}
	}

	view := &PublicView{
		Status:        req.Status,
		RecipientName: req.RecipientName,
		ExpiresAt:     req.ExpiresAt,
		SignedAt:      req.SignedAt,
		SignedName:    req.SignedName,
		DeclinedAt:    req.DeclinedAt,
		Organization:  s.publicOrganization(),
		Booking:       s.bookingFor(req.AppointmentID),
		Items:         make([]PublicItem, 0, len(req.Items)),
	}
	for _, it := range req.Items {
		view.Items = append(view.Items, PublicItem{it.ID, it.Title, it.Kind, it.BodyHTML, it.AcceptedAt})
	}
	return view, nil
}

func (s *Service) publicOrganization() PublicOrganization {
	org := s.organization()
	return PublicOrganization{
		Name:         org.Name,
		Website:      org.Website,
		LogoDataURL:  org.LogoDataURL,
		Phone:        org.Phone,
		SupportEmail: org.SupportEmail,
		Address:      org.Address,
	}
}

func (s *Service) bookingFor(appointmentID *uint) *PublicBooking {
	if appointmentID == nil {
		return nil
	}
	var appt appointments.Appointment
	if s.db.First(&appt, *appointmentID).Error != nil {
		return nil
	}
	booking := &PublicBooking{
		ScheduledAt: appt.ScheduledAt,
		Duration:    appt.Duration,
		ExamFee:     appt.ExamFee,
		Collected:   appt.CollectedAmount,
		Currency:    appt.FeeCurrency,
	}
	if appt.ExamTypeID != nil {
		var et exams.ExamType
		if s.db.Select("name").First(&et, *appt.ExamTypeID).Error == nil {
			booking.ExamType = et.Name
		}
	}
	return booking
}

type SignInput struct {
	SignedName       string `json:"signed_name"`
	SignatureDataURL string `json:"signature_data_url"`
	AcceptedItemIDs  []uint `json:"accepted_item_ids"`
}

func (s *Service) Sign(token string, input SignInput, ip, userAgent string) (*PublicView, error) {
	req, err := s.loadByToken(token)
	if err != nil {
		return nil, err
	}
	if !req.isOpen() {
		return nil, errors.New("these agreements have already been " + req.Status)
	}
	if contentHash(req.Items) != req.ContentHash {
		return nil, errors.New("these agreements could not be verified. Please contact us for a new link")
	}

	accepted := map[uint]bool{}
	for _, id := range input.AcceptedItemIDs {
		accepted[id] = true
	}
	for _, it := range req.Items {
		if !accepted[it.ID] {
			return nil, errors.New("please tick \"I agree\" on every agreement: " + it.Title)
		}
	}
	name, err := normalizeSignedName(input.SignedName)
	if err != nil {
		return nil, err
	}
	sigBytes, err := decodeSignature(input.SignatureDataURL)
	if err != nil {
		return nil, err
	}
	if len(userAgent) > 500 {
		userAgent = userAgent[:500]
	}

	now := time.Now().UTC()
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Conditional update: only one concurrent submission can move the request to signed.
		res := tx.Model(&AgreementRequest{}).
			Where("id = ? AND status IN ? AND expires_at > ?", req.ID, []string{StatusSent, StatusViewed}, now).
			Updates(map[string]interface{}{
				"status":             StatusSigned,
				"signed_at":          now,
				"signed_name":        name,
				"signature_data_url": input.SignatureDataURL,
				"signature_hash":     signatureHash(req.ContentHash, name, sigBytes, now),
				"signer_ip":          ip,
				"signer_user_agent":  userAgent,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("these agreements can no longer be signed")
		}
		return tx.Model(&AgreementRequestItem{}).Where("request_id = ?", req.ID).
			Update("accepted_at", now).Error
	})
	if err != nil {
		return nil, err
	}

	req.Status = StatusSigned
	// The signature is already committed; a PDF or email failure here must not undo it.
	// A missing PDF is regenerated on the first download.
	pdfBytes, _, pdfErr := s.ensurePDF(req.ID)
	if pdfErr == nil {
		sendSignedCopyEmail(req, s.organization().Name, pdfBytes)
	}
	notifyStaff(req, StatusSigned)
	return s.GetPublicView(token)
}

func (s *Service) Decline(token string, reason string) (*PublicView, error) {
	req, err := s.loadByToken(token)
	if err != nil {
		return nil, err
	}
	if !req.isOpen() {
		return nil, errors.New("these agreements have already been " + req.Status)
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 2000 {
		reason = reason[:2000]
	}
	now := time.Now().UTC()
	res := s.db.Model(&AgreementRequest{}).
		Where("id = ? AND status IN ?", req.ID, []string{StatusSent, StatusViewed}).
		Updates(map[string]interface{}{
			"status":         StatusDeclined,
			"declined_at":    now,
			"decline_reason": reason,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, errors.New("these agreements can no longer be declined")
	}
	req.Status, req.DeclineReason = StatusDeclined, reason
	notifyStaff(req, StatusDeclined)
	return s.GetPublicView(token)
}
