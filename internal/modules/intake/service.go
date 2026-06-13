package intake

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"my-app/internal/database"
	"my-app/internal/email"
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

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func publicIntakeURL(token string) string {
	base := strings.TrimSpace(os.Getenv("APP_PUBLIC_URL"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	}
	if base == "" {
		base = "http://localhost:3001"
	}
	return strings.TrimRight(base, "/") + "/intake/" + token
}

// SendIntakeRequest creates an intake token for the given client and emails the link.
func (s *Service) SendIntakeRequest(
	clientID uint,
	recipientEmail, recipientName, message, sentByEmail string,
	expiryDays int,
) (*IntakeRequest, error) {
	if expiryDays <= 0 {
		expiryDays = 7
	}

	// Resolve client name.
	apptSvc := appointments.NewService(nil)
	client, err := apptSvc.GetClientByID(fmt.Sprintf("%d", clientID))
	if err != nil {
		return nil, fmt.Errorf("client not found: %w", err)
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	req := &IntakeRequest{
		Token:          token,
		ClientID:       clientID,
		ClientName:     client.Name,
		RecipientEmail: strings.TrimSpace(recipientEmail),
		RecipientName:  strings.TrimSpace(recipientName),
		Message:        strings.TrimSpace(message),
		ExpiresAt:      time.Now().UTC().Add(time.Duration(expiryDays) * 24 * time.Hour),
		Status:         "pending",
		SentByEmail:    sentByEmail,
	}
	if err := s.db.Create(req).Error; err != nil {
		return nil, err
	}

	// Send email.
	link := publicIntakeURL(token)
	name := req.RecipientName
	if name == "" {
		name = req.RecipientEmail
	}

	customMsg := ""
	if req.Message != "" {
		customMsg = "\n\n" + req.Message
	}

	body := fmt.Sprintf(
		"Hello %s,\n\nPolygraph Forensic System requires the names and details of the individuals from %s who will be examined.\n\nPlease use the secure link below to submit their information. The link expires on %s.%s\n\n%s\n\nIf you have any questions, please reply to this email.\n\nThank you,\nPolygraph Team",
		name,
		client.Name,
		req.ExpiresAt.Format("2 January 2006"),
		customMsg,
		link,
	)

	if err := sendSMTPMail(req.RecipientEmail, "Action required: Submit examinee details for "+client.Name, body); err != nil {
		// Still return the request — admin can copy/resend the link manually.
		return req, fmt.Errorf("intake request created but email failed: %w", err)
	}

	return req, nil
}

// GetPublicRequest returns the intake request for a token, checking expiry.
func (s *Service) GetPublicRequest(token string) (*IntakeRequest, error) {
	var req IntakeRequest
	if err := s.db.Where("token = ?", token).First(&req).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("intake form not found")
		}
		return nil, err
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		return nil, errors.New("this intake link has expired")
	}
	if req.Status == "submitted" {
		return nil, errors.New("this intake form has already been submitted")
	}
	return &req, nil
}

// SubmitIntakeRequest creates subjects under the client from the submitted rows.
// agreed must be true: the submitter has to accept the accuracy + data-use declaration.
func (s *Service) SubmitIntakeRequest(token string, rows []SubjectInput, agreed bool) ([]subjects.Subject, error) {
	if !agreed {
		return nil, errors.New("you must accept the declaration before submitting")
	}

	var req IntakeRequest
	if err := s.db.Where("token = ?", token).First(&req).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("intake form not found")
		}
		return nil, err
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		return nil, errors.New("this intake link has expired")
	}
	if req.Status == "submitted" {
		return nil, errors.New("this intake form has already been submitted")
	}

	created := make([]subjects.Subject, 0, len(rows))

	err := s.db.Transaction(func(tx *gorm.DB) error {
		for i, row := range rows {
			first := strings.TrimSpace(row.FirstName)
			last := strings.TrimSpace(row.LastName)
			if first == "" && last == "" {
				continue
			}
			if first == "" {
				first = "Examinee"
			}
			if last == "" {
				last = "Record"
			}
			subj := subjects.Subject{
				ClientID:    &req.ClientID,
				FirstName:   first,
				LastName:    last,
				Email:       strings.TrimSpace(row.Email),
				Phone:       strings.TrimSpace(row.Phone),
				EmployeeRef: strings.TrimSpace(row.EmployeeRef),
				Nationality: strings.TrimSpace(row.Nationality),
				Gender:      strings.TrimSpace(row.Gender),
			}
			if err := tx.Create(&subj).Error; err != nil {
				return fmt.Errorf("row %d (%s %s): %w", i+1, first, last, err)
			}
			created = append(created, subj)
		}

		if len(created) == 0 {
			return errors.New("no valid rows submitted")
		}

		// Serialise created subjects for audit trail.
		b, _ := json.Marshal(created)
		now := time.Now().UTC()
		return tx.Model(&req).Updates(map[string]interface{}{
			"status":           "submitted",
			"submitted_at":     &now,
			"agreed_at":        &now,
			"created_subjects": string(b),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// ResendIntakeRequest re-sends the original intake link email for a pending request.
func (s *Service) ResendIntakeRequest(id string) (*IntakeRequest, error) {
	var req IntakeRequest
	if err := s.db.First(&req, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("intake request not found")
		}
		return nil, err
	}
	if req.Status == "submitted" {
		return nil, errors.New("this intake form has already been submitted")
	}
	if time.Now().UTC().After(req.ExpiresAt) {
		return nil, errors.New("this intake link has expired; send a new request")
	}

	link := publicIntakeURL(req.Token)
	name := req.RecipientName
	if name == "" {
		name = req.RecipientEmail
	}
	customMsg := ""
	if req.Message != "" {
		customMsg = "\n\n" + req.Message
	}
	body := fmt.Sprintf(
		"Hello %s,\n\nThis is a reminder that Polygraph Forensic System requires the names and details of the individuals from %s who will be examined.\n\nPlease use the secure link below to submit their information. The link expires on %s.%s\n\n%s\n\nIf you have any questions, please reply to this email.\n\nThank you,\nPolygraph Team",
		name,
		req.ClientName,
		req.ExpiresAt.Format("2 January 2006"),
		customMsg,
		link,
	)
	if err := sendSMTPMail(req.RecipientEmail, "Reminder: Submit examinee details for "+req.ClientName, body); err != nil {
		return nil, fmt.Errorf("failed to resend email: %w", err)
	}
	return &req, nil
}

// IntakeSubmission is the read-only view of what an organisation submitted.
type IntakeSubmission struct {
	ID          uint              `json:"id"`
	ClientID    uint              `json:"client_id"`
	ClientName  string            `json:"client_name"`
	Status      string            `json:"status"`
	SubmittedAt *time.Time        `json:"submitted_at,omitempty"`
	AgreedAt    *time.Time        `json:"agreed_at,omitempty"`
	Subjects    []subjects.Subject `json:"subjects"`
}

// GetSubmission returns the examinees an organisation submitted plus consent metadata.
func (s *Service) GetSubmission(id string) (*IntakeSubmission, error) {
	var req IntakeRequest
	if err := s.db.First(&req, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("intake request not found")
		}
		return nil, err
	}
	if req.Status != "submitted" {
		return nil, errors.New("this intake form has not been submitted yet")
	}

	var subs []subjects.Subject
	if strings.TrimSpace(req.CreatedSubjects) != "" {
		// Stored as a JSON snapshot at submission time.
		_ = json.Unmarshal([]byte(req.CreatedSubjects), &subs)
	}

	return &IntakeSubmission{
		ID:          req.ID,
		ClientID:    req.ClientID,
		ClientName:  req.ClientName,
		Status:      req.Status,
		SubmittedAt: req.SubmittedAt,
		AgreedAt:    req.AgreedAt,
		Subjects:    subs,
	}, nil
}

// ListIntakeRequests returns all intake requests, newest first.
func (s *Service) ListIntakeRequests(clientID string) ([]IntakeRequest, error) {
	var list []IntakeRequest
	q := s.db.Order("created_at DESC")
	if strings.TrimSpace(clientID) != "" {
		q = q.Where("client_id = ?", clientID)
	}
	return list, q.Find(&list).Error
}

// ─── SMTP helper ─────────────────────────────────────────────────────────────

func sendSMTPMail(toEmail, subject, body string) error {
	return email.Send(toEmail, subject, body)
}

// SubjectInput is one row submitted by the organisation.
type SubjectInput struct {
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	EmployeeRef string `json:"employee_ref"`
	Nationality string `json:"nationality"`
	Gender      string `json:"gender"`
}
