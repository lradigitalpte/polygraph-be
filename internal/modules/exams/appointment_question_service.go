package exams

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"my-app/internal/modules/subjects"
)

// errDocumentationStarted is returned when a booking's prepared questions are
// edited after its examination record exists. From that point the ExamQuestion
// rows are the session's authoritative record and these become frozen history.
var errDocumentationStarted = errors.New("documentation has already started; edit the session's questions instead")

type AppointmentQuestionInput struct {
	Text     string `json:"text"`
	Category string `json:"category"`
}

func (s *Service) ListAppointmentQuestions(appointmentID uint) ([]AppointmentQuestion, error) {
	var questions []AppointmentQuestion
	if err := s.db.Where("appointment_id = ?", appointmentID).
		Order("sort_order ASC, id ASC").
		Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}

// loadEditableAppointment fetches the booking and refuses if documentation has
// already started.
func (s *Service) loadEditableAppointment(appointmentID uint) (*appointmentLink, error) {
	var appt appointmentLink
	if err := s.db.First(&appt, appointmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("appointment not found")
		}
		return nil, err
	}
	if appt.ExamID != nil && *appt.ExamID > 0 {
		return nil, errDocumentationStarted
	}
	return &appt, nil
}

// ReplaceAppointmentQuestions overwrites the booking's prepared questions with
// the given list, in the order supplied. A full replace keeps the editor simple:
// the UI edits, reorders and deletes locally, then saves once.
func (s *Service) ReplaceAppointmentQuestions(appointmentID uint, items []AppointmentQuestionInput) ([]AppointmentQuestion, error) {
	if _, err := s.loadEditableAppointment(appointmentID); err != nil {
		return nil, err
	}

	rows := make([]AppointmentQuestion, 0, len(items))
	for i, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			return nil, errors.New("question text cannot be empty")
		}
		category := strings.ToLower(strings.TrimSpace(item.Category))
		if category != "" && !validQuestionCategories[category] {
			return nil, errors.New("category must be one of: relevant, comparison, irrelevant")
		}
		rows = append(rows, AppointmentQuestion{
			AppointmentID: appointmentID,
			Text:          text,
			Category:      category,
			SortOrder:     i,
		})
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("appointment_id = ?", appointmentID).Delete(&AppointmentQuestion{}).Error; err != nil {
			return err
		}
		for i := range rows {
			if err := tx.Create(&rows[i]).Error; err != nil {
				return err
			}
		}
		return tx.Model(&appointmentLink{}).
			Where("id = ?", appointmentID).
			Update("questions_prepared", len(rows) > 0).Error
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

// AddAppointmentQuestionsFromTemplates appends library questions to a booking,
// resolving merge fields such as {{subject_name}} against that booking.
func (s *Service) AddAppointmentQuestionsFromTemplates(appointmentID uint, templateIDs []uint) ([]AppointmentQuestion, error) {
	if len(templateIDs) == 0 {
		return nil, errors.New("no templates selected")
	}
	appt, err := s.loadEditableAppointment(appointmentID)
	if err != nil {
		return nil, err
	}

	var templates []QuestionTemplate
	if err := s.db.Where("id IN ?", templateIDs).Order("sort_order ASC, id ASC").Find(&templates).Error; err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, errors.New("no matching question templates found")
	}

	var result struct{ Max *int }
	if err := s.db.Model(&AppointmentQuestion{}).
		Select("MAX(sort_order) AS max").
		Where("appointment_id = ?", appointmentID).
		Scan(&result).Error; err != nil {
		return nil, err
	}
	nextOrder := 0
	if result.Max != nil {
		nextOrder = *result.Max + 1
	}

	ctx := s.mergeContextForAppointment(appt)

	var created []AppointmentQuestion
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for i, tpl := range templates {
			question := AppointmentQuestion{
				AppointmentID: appointmentID,
				Text:          mergeTemplatePlaceholders(tpl.Text, ctx),
				Category:      tpl.Category,
				SortOrder:     nextOrder + i,
			}
			if err := tx.Create(&question).Error; err != nil {
				return err
			}
			created = append(created, question)
		}
		return tx.Model(&appointmentLink{}).
			Where("id = ?", appointmentID).
			Update("questions_prepared", true).Error
	}); err != nil {
		return nil, err
	}
	return created, nil
}

// mergeContextForAppointment mirrors mergeContextForExam for a booking that has
// no examination record yet.
func (s *Service) mergeContextForAppointment(appt *appointmentLink) ReportMergeContext {
	subjectName := ""
	subjectGender := ""
	if appt.SubjectID > 0 {
		var subject subjects.Subject
		if err := s.db.First(&subject, appt.SubjectID).Error; err == nil {
			subjectName = strings.TrimSpace(subject.FirstName + " " + subject.LastName)
			subjectGender = subject.Gender
		}
	}
	clientName := ""
	if appt.ClientID > 0 {
		type clientRow struct{ Name string }
		var row clientRow
		if err := s.db.Table("clients").Select("name").Where("id = ?", appt.ClientID).Scan(&row).Error; err == nil {
			clientName = row.Name
		}
	}
	examDate := ""
	if !appt.ScheduledAt.IsZero() {
		examDate = appt.ScheduledAt.Format("January 2, 2006")
	}
	return ReportMergeContext{
		SubjectName:   subjectName,
		ClientName:    clientName,
		ExamDate:      examDate,
		SubjectGender: subjectGender,
	}
}
