package leads

import (
	"fmt"

	"my-app/internal/database"

	"gorm.io/gorm"
)

// Service handles database operations for leads.
type Service struct {
	db *gorm.DB
}

// NewService creates a Service backed by the shared database connection.
func NewService() *Service {
	return &Service{db: database.GetDB()}
}

// Create inserts a new lead, generating its intake reference beforehand.
func (s *Service) Create(input *CreateLeadInput) (*Lead, error) {
	lead := &Lead{
		Name:             input.Name,
		Email:            input.Email,
		Phone:            input.Phone,
		Source:           input.Source,
		Interest:         input.Interest,
		Notes:            input.Notes,
		PreferredContact: input.PreferredContact,
		Priority:         input.Priority,
		EstimatedValue:   input.EstimatedValue,
		NextStep:         input.NextStep,
		Status:           StatusNew,
	}

	if lead.Priority == "" {
		lead.Priority = PriorityStandard
	}

	// Generate ref inside a transaction so count is consistent
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Lead{}).Count(&count).Error; err != nil {
			return err
		}

		lead.Ref = fmt.Sprintf("LD-%04d", count+1)

		return tx.Create(lead).Error
	})

	if err != nil {
		return nil, err
	}

	return lead, nil
}

// GetAll returns every lead ordered by most recently created first.
func (s *Service) GetAll() ([]Lead, error) {
	var leads []Lead
	err := s.db.Order("created_at desc").Find(&leads).Error
	return leads, err
}

// GetByID returns a single lead by its primary key.
func (s *Service) GetByID(id uint) (*Lead, error) {
	var lead Lead
	if err := s.db.First(&lead, id).Error; err != nil {
		return nil, err
	}

	return &lead, nil
}

// Update applies a partial patch to an existing lead.
func (s *Service) Update(id uint, input *UpdateLeadInput) (*Lead, error) {
	lead, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	if input.Status != nil {
		lead.Status = *input.Status
	}

	if input.Priority != nil {
		lead.Priority = *input.Priority
	}

	if input.Notes != "" {
		lead.Notes = input.Notes
	}

	if input.NextStep != "" {
		lead.NextStep = input.NextStep
	}

	if input.EstimatedValue != nil {
		lead.EstimatedValue = *input.EstimatedValue
	}

	if err := s.db.Save(lead).Error; err != nil {
		return nil, err
	}

	return lead, nil
}

// Delete soft-deletes a lead by its primary key.
func (s *Service) Delete(id uint) error {
	return s.db.Delete(&Lead{}, id).Error
}
