package exams

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_QuestionTemplateCRUD(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})

	examType := ExamType{Name: "Specific Issue", Duration: 150, Active: true}
	require.NoError(t, db.Create(&examType).Error)

	created, err := s.CreateQuestionTemplate(QuestionTemplateInput{
		ExamTypeID: examType.ID,
		Category:   "relevant",
		Text:       "Did you take the missing funds?",
		SortOrder:  1,
	})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.True(t, created.Active)

	_, err = s.CreateQuestionTemplate(QuestionTemplateInput{
		ExamTypeID: examType.ID,
		Category:   "not-a-category",
		Text:       "Bad category",
	})
	assert.Error(t, err)

	_, err = s.CreateQuestionTemplate(QuestionTemplateInput{
		ExamTypeID: 0,
		Category:   "relevant",
		Text:       "No exam type",
	})
	assert.Error(t, err)

	list, err := s.ListQuestionTemplates(&examType.ID, false)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	updated, err := s.UpdateQuestionTemplate(created.ID, QuestionTemplateInput{
		Text: "Did you steal the missing funds?",
	})
	require.NoError(t, err)
	assert.Equal(t, "Did you steal the missing funds?", updated.Text)
	assert.Equal(t, "relevant", updated.Category) // untouched fields preserved

	require.NoError(t, s.DeactivateQuestionTemplate(created.ID))
	activeOnly, err := s.ListQuestionTemplates(&examType.ID, false)
	require.NoError(t, err)
	assert.Len(t, activeOnly, 0)

	withInactive, err := s.ListQuestionTemplates(&examType.ID, true)
	require.NoError(t, err)
	assert.Len(t, withInactive, 1)
}
