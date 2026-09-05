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
	ExamTypeID uint   `json:"exam_type_id"`
	Category   string `json:"category"`
	Text       string `json:"text"`
	SortOrder  int    `json:"sort_order"`
	Active     bool   `json:"active"`
}

func (s *Service) ListQuestionTemplates(examTypeID *uint, includeInactive bool) ([]QuestionTemplate, error) {
	var templates []QuestionTemplate
	query := s.db.Order("exam_type_id ASC, sort_order ASC, id ASC")
	if !includeInactive {
		query = query.Where("active = ?", true)
	}
	if examTypeID != nil {
		query = query.Where("exam_type_id = ?", *examTypeID)
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
	if input.ExamTypeID == 0 {
		return nil, errors.New("exam_type_id is required")
	}
	if !validQuestionCategories[input.Category] {
		return nil, errors.New("category must be one of: relevant, comparison, irrelevant")
	}
	if input.Text == "" {
		return nil, errors.New("text is required")
	}
	var examType ExamType
	if err := s.db.First(&examType, input.ExamTypeID).Error; err != nil {
		return nil, errors.New("exam type not found")
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

func (s *Service) UpdateQuestionTemplate(id uint, input QuestionTemplateInput) (*QuestionTemplate, error) {
	var template QuestionTemplate
	if err := s.db.First(&template, id).Error; err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if input.ExamTypeID != 0 {
		var examType ExamType
		if err := s.db.First(&examType, input.ExamTypeID).Error; err != nil {
			return nil, errors.New("exam type not found")
		}
		updates["exam_type_id"] = input.ExamTypeID
	}
	if category := strings.ToLower(strings.TrimSpace(input.Category)); category != "" {
		if !validQuestionCategories[category] {
			return nil, errors.New("category must be one of: relevant, comparison, irrelevant")
		}
		updates["category"] = category
	}
	if text := strings.TrimSpace(input.Text); text != "" {
		updates["text"] = text
	}
	updates["sort_order"] = input.SortOrder
	updates["active"] = input.Active

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
