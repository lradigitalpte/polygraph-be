package exams

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type ReportTemplateInput struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	ContentJSON string `json:"content_json"`
	IsDefault   bool   `json:"is_default"`
	Active      bool   `json:"active"`
}

func (s *Service) ListReportTemplates(includeInactive bool) ([]ReportTemplate, error) {
	var templates []ReportTemplate
	query := s.db.Order("category ASC, name ASC")
	if !includeInactive {
		query = query.Where("active = ?", true)
	}
	if err := query.Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

func (s *Service) GetReportTemplateByID(id uint) (*ReportTemplate, error) {
	var template ReportTemplate
	if err := s.db.First(&template, id).Error; err != nil {
		return nil, err
	}
	return &template, nil
}

func (s *Service) ResolveReportTemplate(clientID uint) (*ReportTemplate, error) {
	if clientID > 0 {
		type clientRow struct {
			DefaultReportTemplateID *uint
		}
		var row clientRow
		if err := s.db.Table("clients").Select("default_report_template_id").Where("id = ?", clientID).Scan(&row).Error; err == nil {
			if row.DefaultReportTemplateID != nil && *row.DefaultReportTemplateID > 0 {
				var tpl ReportTemplate
				if err := s.db.Where("id = ? AND active = ?", *row.DefaultReportTemplateID, true).First(&tpl).Error; err == nil {
					return &tpl, nil
				}
			}
		}
	}

	var tpl ReportTemplate
	if err := s.db.Where("is_default = ? AND active = ?", true, true).First(&tpl).Error; err == nil {
		return &tpl, nil
	}
	if err := s.db.Where("active = ?", true).Order("id ASC").First(&tpl).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (s *Service) CreateReportTemplate(input ReportTemplateInput) (*ReportTemplate, error) {
	input.Slug = strings.TrimSpace(strings.ToLower(input.Slug))
	input.Name = strings.TrimSpace(input.Name)
	if input.Slug == "" || input.Name == "" {
		return nil, errors.New("slug and name are required")
	}
	if strings.TrimSpace(input.ContentJSON) == "" {
		return nil, errors.New("content_json is required")
	}
	if _, err := parseTemplateContent(input.ContentJSON); err != nil {
		return nil, fmt.Errorf("invalid content_json: %w", err)
	}
	if input.Category == "" {
		input.Category = "generic"
	}

	template := ReportTemplate{
		Slug:        input.Slug,
		Name:        input.Name,
		Category:    input.Category,
		Description: strings.TrimSpace(input.Description),
		ContentJSON: input.ContentJSON,
		IsDefault:   input.IsDefault,
		Active:      input.Active,
	}
	if !template.Active {
		template.Active = true
	}

	return &template, s.db.Transaction(func(tx *gorm.DB) error {
		if template.IsDefault {
			if err := tx.Model(&ReportTemplate{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&template).Error
	})
}

func (s *Service) UpdateReportTemplate(id uint, input ReportTemplateInput) (*ReportTemplate, error) {
	var template ReportTemplate
	if err := s.db.First(&template, id).Error; err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if slug := strings.TrimSpace(strings.ToLower(input.Slug)); slug != "" {
		updates["slug"] = slug
	}
	if name := strings.TrimSpace(input.Name); name != "" {
		updates["name"] = name
	}
	if category := strings.TrimSpace(input.Category); category != "" {
		updates["category"] = category
	}
	updates["description"] = strings.TrimSpace(input.Description)
	if strings.TrimSpace(input.ContentJSON) != "" {
		if _, err := parseTemplateContent(input.ContentJSON); err != nil {
			return nil, fmt.Errorf("invalid content_json: %w", err)
		}
		updates["content_json"] = input.ContentJSON
	}
	updates["active"] = input.Active

	return &template, s.db.Transaction(func(tx *gorm.DB) error {
		if input.IsDefault {
			if err := tx.Model(&ReportTemplate{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
			updates["is_default"] = true
		} else if !input.IsDefault && template.IsDefault {
			updates["is_default"] = false
		}
		if err := tx.Model(&template).Updates(updates).Error; err != nil {
			return err
		}
		return tx.First(&template, id).Error
	})
}

func (s *Service) DeactivateReportTemplate(id uint) error {
	var template ReportTemplate
	if err := s.db.First(&template, id).Error; err != nil {
		return err
	}
	if template.IsDefault {
		return errors.New("cannot deactivate the organization default template")
	}
	return s.db.Model(&template).Update("active", false).Error
}

func (s *Service) BuildReportContentFromTemplate(templateID uint, ctx ReportMergeContext) (StructuredReport, error) {
	tpl, err := s.GetReportTemplateByID(templateID)
	if err != nil {
		return StructuredReport{}, err
	}
	data, err := parseTemplateContent(tpl.ContentJSON)
	if err != nil {
		return StructuredReport{}, err
	}
	merged := mergeStructuredReport(data, ctx)
	merged.SourceTemplateID = tpl.ID
	return merged, nil
}

func structuredReportToJSON(data StructuredReport) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
