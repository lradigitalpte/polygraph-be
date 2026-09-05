package exams

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

func (s *Service) ListExamQuestions(examID uint) ([]ExamQuestion, error) {
	var questions []ExamQuestion
	if err := s.db.Where("exam_id = ?", examID).Order("sort_order ASC, id ASC").Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}

// isReportLocked reports whether the exam's forensic report has already been
// locked, in which case its recorded questions must not be mutated further.
func (s *Service) isReportLocked(examID uint) (bool, error) {
	var report ExamReport
	err := s.db.Where("exam_id = ?", examID).First(&report).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return report.IsLocked, nil
}

func (s *Service) CreateExamQuestion(examID uint, text, category string) (*ExamQuestion, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("text is required")
	}
	category = strings.ToLower(strings.TrimSpace(category))
	if category != "" && !validQuestionCategories[category] {
		return nil, errors.New("category must be one of: relevant, comparison, irrelevant")
	}
	locked, err := s.isReportLocked(examID)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, errors.New("cannot modify questions on a locked forensic report")
	}
	sortOrder, err := s.nextExamQuestionSortOrder(examID)
	if err != nil {
		return nil, err
	}
	question := ExamQuestion{ExamID: examID, Text: text, Category: category, SortOrder: sortOrder}
	if err := s.db.Create(&question).Error; err != nil {
		return nil, err
	}
	return &question, nil
}

type ExamQuestionUpdate struct {
	Text      *string `json:"text"`
	Category  *string `json:"category"`
	Response  *string `json:"response"`
	SortOrder *int    `json:"sort_order"`
}

func (s *Service) UpdateExamQuestion(examID, questionID uint, input ExamQuestionUpdate) (*ExamQuestion, error) {
	var question ExamQuestion
	if err := s.db.Where("id = ? AND exam_id = ?", questionID, examID).First(&question).Error; err != nil {
		return nil, errors.New("question not found")
	}
	locked, err := s.isReportLocked(examID)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, errors.New("cannot modify questions on a locked forensic report")
	}

	updates := map[string]interface{}{}
	if input.Text != nil {
		text := strings.TrimSpace(*input.Text)
		if text == "" {
			return nil, errors.New("text cannot be empty")
		}
		updates["text"] = text
	}
	if input.Category != nil {
		category := strings.ToLower(strings.TrimSpace(*input.Category))
		if category != "" && !validQuestionCategories[category] {
			return nil, errors.New("category must be one of: relevant, comparison, irrelevant")
		}
		updates["category"] = category
	}
	if input.Response != nil {
		updates["response"] = strings.TrimSpace(*input.Response)
	}
	if input.SortOrder != nil {
		updates["sort_order"] = *input.SortOrder
	}
	if len(updates) == 0 {
		return &question, nil
	}
	if err := s.db.Model(&question).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &question, s.db.First(&question, question.ID).Error
}

func (s *Service) DeleteExamQuestion(examID, questionID uint) error {
	var question ExamQuestion
	if err := s.db.Where("id = ? AND exam_id = ?", questionID, examID).First(&question).Error; err != nil {
		return errors.New("question not found")
	}
	locked, err := s.isReportLocked(examID)
	if err != nil {
		return err
	}
	if locked {
		return errors.New("cannot modify questions on a locked forensic report")
	}
	return s.db.Delete(&question).Error
}

// AddExamQuestionsFromTemplates appends library questions to an exam that is
// already underway, merge-resolving each template's placeholders.
func (s *Service) AddExamQuestionsFromTemplates(examID uint, templateIDs []uint) ([]ExamQuestion, error) {
	if len(templateIDs) == 0 {
		return nil, errors.New("no templates selected")
	}
	locked, err := s.isReportLocked(examID)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, errors.New("cannot modify questions on a locked forensic report")
	}

	var exam Exam
	if err := s.db.Preload("Subject").First(&exam, examID).Error; err != nil {
		return nil, errors.New("exam not found")
	}

	var templates []QuestionTemplate
	if err := s.db.Where("id IN ?", templateIDs).Order("sort_order ASC, id ASC").Find(&templates).Error; err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, errors.New("no matching question templates found")
	}

	nextOrder, err := s.nextExamQuestionSortOrder(examID)
	if err != nil {
		return nil, err
	}
	ctx := s.mergeContextForExam(&exam)

	var created []ExamQuestion
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		for i, tpl := range templates {
			question := ExamQuestion{
				ExamID:    examID,
				Text:      mergeTemplatePlaceholders(tpl.Text, ctx),
				Category:  tpl.Category,
				SortOrder: nextOrder + i,
			}
			if err := tx.Create(&question).Error; err != nil {
				return err
			}
			created = append(created, question)
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return created, nil
}

func (s *Service) nextExamQuestionSortOrder(examID uint) (int, error) {
	var result struct{ Max *int }
	if err := s.db.Model(&ExamQuestion{}).
		Select("MAX(sort_order) AS max").
		Where("exam_id = ?", examID).
		Scan(&result).Error; err != nil {
		return 0, err
	}
	if result.Max == nil {
		return 0, nil
	}
	return *result.Max + 1, nil
}

// mergeContextForExam builds a ReportMergeContext from an exam record so
// question template placeholders like {{subject_name}} can be resolved.
func (s *Service) mergeContextForExam(exam *Exam) ReportMergeContext {
	clientName := ""
	if exam.ClientID > 0 {
		type clientRow struct{ Name string }
		var row clientRow
		if err := s.db.Table("clients").Select("name").Where("id = ?", exam.ClientID).Scan(&row).Error; err == nil {
			clientName = row.Name
		}
	}
	examDate := ""
	if !exam.Date.IsZero() {
		examDate = exam.Date.Format("January 2, 2006")
	}
	return ReportMergeContext{
		SubjectName:   strings.TrimSpace(exam.Subject.FirstName + " " + exam.Subject.LastName),
		ClientName:    clientName,
		ExamDate:      examDate,
		SubjectGender: exam.Subject.Gender,
	}
}
