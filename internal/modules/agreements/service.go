package agreements

import (
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"my-app/internal/database"
	"my-app/internal/storage"
)

var ErrTemplateNotFound = errors.New("agreement not found")

type Service struct {
	db *gorm.DB
	// storage receives a copy of each signed PDF for the Document Vault; nil skips that copy.
	storage storage.Storage
}

func NewService(store storage.Storage) *Service {
	return &Service{db: database.GetDB(), storage: store}
}

type TemplateInput struct {
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	BodyHTML  string `json:"body_html"`
	Active    *bool  `json:"active"`
	SortOrder *int   `json:"sort_order"`
}

func (s *Service) ListTemplates(includeInactive bool) ([]AgreementTemplate, error) {
	var rows []AgreementTemplate
	q := s.db.Order("sort_order ASC, id ASC")
	if !includeInactive {
		q = q.Where("active = ?", true)
	}
	err := q.Find(&rows).Error
	return rows, err
}

func (s *Service) GetTemplate(id string) (*AgreementTemplate, error) {
	parsed, err := strconv.ParseUint(id, 10, 64)
	if err != nil || parsed == 0 {
		return nil, ErrTemplateNotFound
	}
	var row AgreementTemplate
	if err := s.db.First(&row, uint(parsed)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTemplateNotFound
		}
		return nil, err
	}
	return &row, nil
}

// normalize validates the input and returns the cleaned title, kind and sanitized terms.
func normalize(input TemplateInput) (string, string, string, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return "", "", "", errors.New("title is required")
	}
	if len(title) > 255 {
		return "", "", "", errors.New("title must be 255 characters or fewer")
	}
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind == "" {
		kind = KindCustom
	}
	if !validKinds[kind] {
		return "", "", "", errors.New("type must be payment, reschedule, cancellation or custom")
	}
	body := SanitizeTermsHTML(input.BodyHTML)
	if termsPlainText(body) == "" {
		return "", "", "", errors.New("terms are required")
	}
	return title, kind, body, nil
}

func (s *Service) CreateTemplate(input TemplateInput, actorEmail string) (*AgreementTemplate, error) {
	title, kind, body, err := normalize(input)
	if err != nil {
		return nil, err
	}
	row := AgreementTemplate{
		Title:     title,
		Kind:      kind,
		BodyHTML:  body,
		Version:   1,
		Active:    true,
		UpdatedBy: actorEmail,
	}
	if input.Active != nil {
		row.Active = *input.Active
	}
	if input.SortOrder != nil {
		row.SortOrder = *input.SortOrder
	} else {
		var maxOrder int
		s.db.Model(&AgreementTemplate{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
		row.SortOrder = maxOrder + 1
	}
	// GORM skips zero-value bools on create, so an explicit Active=false needs a follow-up update.
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	if !row.Active {
		if err := s.db.Model(&row).Update("active", false).Error; err != nil {
			return nil, err
		}
	}
	return &row, nil
}

func (s *Service) UpdateTemplate(id string, input TemplateInput, actorEmail string) (*AgreementTemplate, error) {
	row, err := s.GetTemplate(id)
	if err != nil {
		return nil, err
	}
	title, kind, body, err := normalize(input)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{
		"title":      title,
		"kind":       kind,
		"body_html":  body,
		"updated_by": actorEmail,
	}
	// A wording change is a new version; toggling active or reordering is not.
	if title != row.Title || body != row.BodyHTML {
		updates["version"] = row.Version + 1
	}
	if input.Active != nil {
		updates["active"] = *input.Active
	}
	if input.SortOrder != nil {
		updates["sort_order"] = *input.SortOrder
	}
	if err := s.db.Model(row).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetTemplate(id)
}

func (s *Service) SetTemplateActive(id string, active bool) (*AgreementTemplate, error) {
	row, err := s.GetTemplate(id)
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(row).Update("active", active).Error; err != nil {
		return nil, err
	}
	return s.GetTemplate(id)
}

// DeleteTemplate soft-deletes the template. Agreements already sent keep their own copy
// of the terms, so removing a template never affects signed records.
func (s *Service) DeleteTemplate(id string) error {
	row, err := s.GetTemplate(id)
	if err != nil {
		return err
	}
	return s.db.Delete(row).Error
}
