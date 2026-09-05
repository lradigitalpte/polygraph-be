package exams

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

func (s *Service) ListExamQuestions(examID uint) ([]ExamQuestion, error) {
	var questions []ExamQuestion
	if err := s.db.Where("exam_id = ?", examID).Order("id ASC").Find(&questions).Error; err != nil {
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
	question := ExamQuestion{ExamID: examID, Text: text, Category: category}
	if err := s.db.Create(&question).Error; err != nil {
		return nil, err
	}
	return &question, nil
}

type ExamQuestionUpdate struct {
	Text     *string `json:"text"`
	Category *string `json:"category"`
	Response *string `json:"response"`
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

// PopulateDefaultQuestions copies the exam's exam-type default question
// templates onto the exam, merge-field resolving each one's text. It refuses
// to run if the exam already has questions, so an examiner never loses
// session-specific edits by re-populating.
func (s *Service) PopulateDefaultQuestions(examID uint) ([]ExamQuestion, error) {
	var exam Exam
	if err := s.db.Preload("Subject").First(&exam, examID).Error; err != nil {
		return nil, errors.New("exam not found")
	}
	if exam.ExamTypeID == nil || *exam.ExamTypeID == 0 {
		return nil, errors.New("exam has no exam type set")
	}

	var existingCount int64
	if err := s.db.Model(&ExamQuestion{}).Where("exam_id = ?", examID).Count(&existingCount).Error; err != nil {
		return nil, err
	}
	if existingCount > 0 {
		return nil, errors.New("exam already has questions; clear or edit them individually instead of repopulating")
	}

	templates, err := s.ListQuestionTemplates(exam.ExamTypeID, false)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, errors.New("no question templates found for this exam type")
	}

	ctx := s.mergeContextForExam(&exam)

	var created []ExamQuestion
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		for _, tpl := range templates {
			question := ExamQuestion{
				ExamID:   examID,
				Text:     mergeTemplatePlaceholders(tpl.Text, ctx),
				Category: tpl.Category,
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
