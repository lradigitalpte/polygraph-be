package exams

import (
	"testing"

	"my-app/internal/modules/subjects"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_PopulateDefaultQuestionsCopiesAndMergesTemplates(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})

	client := testClient{Name: "Acme Corp"}
	require.NoError(t, db.Create(&client).Error)

	subject := subjects.Subject{FirstName: "Jane", LastName: "Doe", IDNumber: "123456789"}
	require.NoError(t, db.Create(&subject).Error)

	examType := ExamType{Name: "Specific Issue", Duration: 150, Active: true}
	require.NoError(t, db.Create(&examType).Error)

	_, err := s.CreateQuestionTemplate(QuestionTemplateInput{
		ExamTypeID: examType.ID, Category: "relevant", Text: "Did {{subject_name}} take the funds?", SortOrder: 1,
	})
	require.NoError(t, err)
	_, err = s.CreateQuestionTemplate(QuestionTemplateInput{
		ExamTypeID: examType.ID, Category: "comparison", Text: "Before today, have you ever lied to {{client_name}}?", SortOrder: 2,
	})
	require.NoError(t, err)

	exam := Exam{ClientID: client.ID, SubjectID: subject.ID, ExaminerID: 1, ExamTypeID: &examType.ID}
	require.NoError(t, db.Create(&exam).Error)

	created, err := s.PopulateDefaultQuestions(exam.ID)
	require.NoError(t, err)
	require.Len(t, created, 2)
	assert.Equal(t, "Did Jane Doe take the funds?", created[0].Text)
	assert.Equal(t, "relevant", created[0].Category)
	assert.Contains(t, created[1].Text, "Acme Corp")
	assert.Equal(t, "comparison", created[1].Category)

	_, err = s.PopulateDefaultQuestions(exam.ID)
	assert.Error(t, err, "repopulating should be blocked once questions exist")

	examWithoutType := Exam{ClientID: client.ID, SubjectID: subject.ID, ExaminerID: 1}
	require.NoError(t, db.Create(&examWithoutType).Error)
	_, err = s.PopulateDefaultQuestions(examWithoutType.ID)
	assert.Error(t, err, "exam without an exam type should be rejected")
}

func TestService_ExamQuestionCRUDBlockedWhenReportLocked(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})

	exam := &Exam{ClientID: 1, SubjectID: 1, ExaminerID: 1}
	require.NoError(t, s.CreateExam(exam))

	question, err := s.CreateExamQuestion(exam.ID, "Did you do it?", "relevant")
	require.NoError(t, err)

	_, err = s.UpdateExamQuestion(exam.ID, question.ID, ExamQuestionUpdate{Response: strPtr("Truthful")})
	require.NoError(t, err)

	report, err := s.CreateReport(exam.ID, "NDI", "content")
	require.NoError(t, err)
	require.NoError(t, db.Model(report).Update("is_locked", true).Error)

	_, err = s.UpdateExamQuestion(exam.ID, question.ID, ExamQuestionUpdate{Response: strPtr("Deceptive")})
	assert.Error(t, err)

	err = s.DeleteExamQuestion(exam.ID, question.ID)
	assert.Error(t, err)

	_, err = s.CreateExamQuestion(exam.ID, "New question after lock", "relevant")
	assert.Error(t, err)
}

func strPtr(s string) *string { return &s }
