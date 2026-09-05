package exams

import (
	"errors"
	"strings"
)

var validQuestionCategories = map[string]bool{
	"relevant":   true,
	"comparison": true,
	"irrelevant": true,
}

type QuestionTemplateInput struct {
	ExamTypeID *uint  `json:"exam_type_id"`
	Category   string `json:"category"`
	Text       string `json:"text"`
	SortOrder  int    `json:"sort_order"`
	Active     bool   `json:"active"`
}

// ListQuestionTemplates returns library questions. When examTypeID is set the
// result is what that booking may use: templates tagged with that exam type
// plus untagged ones, which are offered for every booking.
func (s *Service) ListQuestionTemplates(examTypeID *uint, includeInactive bool) ([]QuestionTemplate, error) {
	var templates []QuestionTemplate
	query := s.db.Order("sort_order ASC, id ASC")
	if !includeInactive {
		query = query.Where("active = ?", true)
	}
	if examTypeID != nil {
		query = query.Where("exam_type_id = ? OR exam_type_id IS NULL", *examTypeID)
	}
	if err := query.Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

func (s *Service) GetQuestionTemplateByID(id uint) (*QuestionTemplate, error) {
	var template QuestionTemplate
	if err := s.db.First(&template, id).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

func (s *Service) CreateQuestionTemplate(input QuestionTemplateInput) (*QuestionTemplate, error) {
	input.Category = strings.ToLower(strings.TrimSpace(input.Category))
	input.Text = strings.TrimSpace(input.Text)
	if !validQuestionCategories[input.Category] {
		return nil, errors.New("category must be one of: relevant, comparison, irrelevant")
	}
	if input.Text == "" {
		return nil, errors.New("text is required")
	}
	// The exam type is an optional tag; validate it only when one was given.
	if input.ExamTypeID != nil && *input.ExamTypeID > 0 {
		var examType ExamType
		if err := s.db.First(&examType, *input.ExamTypeID).Error; err != nil {
			return nil, errors.New("exam type not found")
		}
	} else {
		input.ExamTypeID = nil
	}

	template := QuestionTemplate{
		ExamTypeID: input.ExamTypeID,
		Category:   input.Category,
		Text:       input.Text,
		SortOrder:  input.SortOrder,
		Active:     true,
	}
	if err := s.db.Create(&template).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

// QuestionTemplateUpdate is a partial update: every field is optional, so a
// caller that only reorders rows cannot accidentally blank the others.
// ExamTypeID follows the usual convention here — nil leaves the tag alone,
// while an explicit 0 clears it ("Any exam type").
type QuestionTemplateUpdate struct {
	ExamTypeID *uint   `json:"exam_type_id"`
	Category   *string `json:"category"`
	Text       *string `json:"text"`
	SortOrder  *int    `json:"sort_order"`
	Active     *bool   `json:"active"`
}

func (s *Service) UpdateQuestionTemplate(id uint, input QuestionTemplateUpdate) (*QuestionTemplate, error) {
	var template QuestionTemplate
	if err := s.db.First(&template, id).Error; err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if input.ExamTypeID != nil {
		if *input.ExamTypeID == 0 {
			updates["exam_type_id"] = nil
		} else {
			var examType ExamType
			if err := s.db.First(&examType, *input.ExamTypeID).Error; err != nil {
				return nil, errors.New("exam type not found")
			}
			updates["exam_type_id"] = *input.ExamTypeID
		}
	}
	if input.Category != nil {
		category := strings.ToLower(strings.TrimSpace(*input.Category))
		if !validQuestionCategories[category] {
			return nil, errors.New("category must be one of: relevant, comparison, irrelevant")
		}
		updates["category"] = category
	}
	if input.Text != nil {
		text := strings.TrimSpace(*input.Text)
		if text == "" {
			return nil, errors.New("text cannot be empty")
		}
		updates["text"] = text
	}
	if input.SortOrder != nil {
		updates["sort_order"] = *input.SortOrder
	}
	if input.Active != nil {
		updates["active"] = *input.Active
	}
	if len(updates) == 0 {
		return &template, nil
	}

	if err := s.db.Model(&template).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &template, s.db.First(&template, id).Error
}

func (s *Service) DeactivateQuestionTemplate(id uint) error {
	var template QuestionTemplate
	if err := s.db.First(&template, id).Error; err != nil {
		return err
	}
	return s.db.Model(&template).Update("active", false).Error
}
