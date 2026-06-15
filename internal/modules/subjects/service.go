package subjects

import (
	"errors"
	"strings"

	"my-app/internal/database"

	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService() *Service {
	return &Service{db: database.GetDB()}
}

func (s *Service) Create(subject *Subject) error {
	return s.db.Create(subject).Error
}

func (s *Service) GetAll(search string, clientID string) ([]Subject, error) {
	var subjects []Subject
	query := s.db.Order("first_name ASC, last_name ASC")
	if trimmed := strings.TrimSpace(strings.ToLower(search)); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where(
			"LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ? OR LOWER(email) LIKE ? OR LOWER(phone) LIKE ? OR LOWER(employee_ref) LIKE ?",
			like, like, like, like, like,
		)
	}
	if trimmedClient := strings.TrimSpace(clientID); trimmedClient != "" {
		query = query.Where("client_id = ?", trimmedClient)
	}
	err := query.Find(&subjects).Error
	return subjects, err
}

func (s *Service) GetByID(id string) (*Subject, error) {
	var subject Subject
	if err := s.db.First(&subject, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("subject not found")
		}
		return nil, err
	}
	return &subject, nil
}

// GetAllForExaminer lists only examinees the examiner has an appointment with.
func (s *Service) GetAllForExaminer(examinerID uint, search string, clientID string) ([]Subject, error) {
	var subjects []Subject
	owned := s.db.Table("appointments").Select("subject_id").Where("examiner_id = ?", examinerID)
	query := s.db.Order("first_name ASC, last_name ASC").Where("id IN (?)", owned)
	if trimmed := strings.TrimSpace(strings.ToLower(search)); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where(
			"LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ? OR LOWER(email) LIKE ? OR LOWER(phone) LIKE ? OR LOWER(employee_ref) LIKE ?",
			like, like, like, like, like,
		)
	}
	if trimmedClient := strings.TrimSpace(clientID); trimmedClient != "" {
		query = query.Where("client_id = ?", trimmedClient)
	}
	err := query.Find(&subjects).Error
	return subjects, err
}

// ExaminerOwnsSubject reports whether the examiner has any appointment with the subject.
func (s *Service) ExaminerOwnsSubject(examinerID uint, subjectID string) bool {
	var count int64
	s.db.Table("appointments").Where("examiner_id = ? AND subject_id = ?", examinerID, subjectID).Count(&count)
	return count > 0
}

func (s *Service) Update(id string, input *Subject) error {
	var existing Subject
	if err := s.db.First(&existing, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("subject not found")
		}
		return err
	}

	existing.FirstName = strings.TrimSpace(input.FirstName)
	existing.LastName = strings.TrimSpace(input.LastName)
	existing.Gender = strings.TrimSpace(input.Gender)
	existing.Nationality = strings.TrimSpace(input.Nationality)
	existing.Email = strings.TrimSpace(input.Email)
	existing.Phone = strings.TrimSpace(input.Phone)
	existing.EmployeeRef = strings.TrimSpace(input.EmployeeRef)
	existing.SpokenLanguage = strings.TrimSpace(input.SpokenLanguage)
	existing.WrittenLanguage = strings.TrimSpace(input.WrittenLanguage)
	existing.EnglishProficiency = strings.TrimSpace(input.EnglishProficiency)
	existing.InterpreterRequired = input.InterpreterRequired
	if input.IDNumber != "" || input.DateOfBirth != nil {
		existing.IDNumber = input.IDNumber
		existing.DateOfBirth = input.DateOfBirth
	}
	if input.ClientID != nil {
		existing.ClientID = input.ClientID
	}

	return s.db.Save(&existing).Error
}
