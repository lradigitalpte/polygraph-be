package settings

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"my-app/internal/database"
	"my-app/internal/dbseed"
	"my-app/internal/models"
	"my-app/internal/modules/appointments"
	"my-app/internal/modules/availability"
	"my-app/internal/modules/exams"
	"my-app/internal/modules/forms"
	"my-app/internal/modules/intake"
	"my-app/internal/modules/inventory"
	"my-app/internal/modules/leads"
	"my-app/internal/modules/subjects"
)

const singletonID uint = 1

type Service struct {
	db *gorm.DB
}

type UpdateOrganizationInput struct {
	Name         string   `json:"name"`
	SupportEmail string   `json:"support_email"`
	Address      string   `json:"address"`
	Currency     string   `json:"currency"`
	UsdAedRate   *float64 `json:"usd_aed_rate"`
	UsdGbpRate   *float64 `json:"usd_gbp_rate"`
	UsdEurRate   *float64 `json:"usd_eur_rate"`
}

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

func (s *Service) GetOrganization() (*OrganizationSettings, error) {
	var row OrganizationSettings
	err := s.db.First(&row, singletonID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = OrganizationSettings{
			ID:         singletonID,
			Name:       "Polygraph Forensic Labs",
			Currency:   "USD",
			UsdAedRate: 3.6725,
			UsdGbpRate: 0.7850,
			UsdEurRate: 0.9250,
		}
		if createErr := s.db.Create(&row).Error; createErr != nil {
			return nil, createErr
		}
		return &row, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) UpdateOrganization(input UpdateOrganizationInput) (*OrganizationSettings, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("organization name is required")
	}

	row, err := s.GetOrganization()
	if err != nil {
		return nil, err
	}

	currency := strings.TrimSpace(input.Currency)
	if currency == "" {
		currency = "USD"
	}

	updates := map[string]interface{}{
		"name":          name,
		"support_email": strings.TrimSpace(input.SupportEmail),
		"address":       strings.TrimSpace(input.Address),
		"currency":      currency,
	}
	if input.UsdAedRate != nil {
		updates["usd_aed_rate"] = *input.UsdAedRate
	}
	if input.UsdGbpRate != nil {
		updates["usd_gbp_rate"] = *input.UsdGbpRate
	}
	if input.UsdEurRate != nil {
		updates["usd_eur_rate"] = *input.UsdEurRate
	}
	if err := s.db.Model(row).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetOrganization()
}

func (s *Service) DeleteOrganizationData() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Break optional FK links that block hard deletes (exam <-> appointment cycle, etc.).
		nullableFKs := []struct {
			model  interface{}
			column string
		}{
			{&appointments.Appointment{}, "exam_id"},
			{&exams.Exam{}, "appointment_id"},
			{&appointments.Quotation{}, "appointment_id"},
			{&forms.FormRequest{}, "client_document_id"},
			{&forms.FormRequest{}, "subject_document_id"},
			{&forms.FormRequest{}, "subject_id"},
		}
		for _, fk := range nullableFKs {
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).
				Model(fk.model).
				Where(fk.column + " IS NOT NULL").
				Update(fk.column, nil).Error; err != nil {
				return err
			}
		}

		tables := []interface{}{
			&forms.FormRequest{},
			&intake.IntakeRequest{},
			&appointments.DocumentShare{},
			&appointments.SubjectDocument{},
			&appointments.ClientDocument{},
			&exams.Document{},
			&exams.SecureReportShare{},
			&exams.ExamReport{},
			&exams.ExamQuestion{},
			&exams.ExamPhase{},
			&exams.ClinicalAssessment{},
			&exams.CaseReferral{},
			&exams.Exam{},
			&appointments.Quotation{},
			&appointments.Appointment{},
			&subjects.Subject{},
			&appointments.Client{},
			&leads.Lead{},
			&availability.Block{},
			&inventory.InventoryItem{},
			&models.AuditLog{},
			&exams.ExamType{},
			&forms.FormTemplate{},
		}
		for _, table := range tables {
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table).Error; err != nil {
				return err
			}
		}

		forms.SeedTemplates(tx)
		dbseed.SeedExamTypes(tx)

		return tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&OrganizationSettings{}).Error
	})
}
