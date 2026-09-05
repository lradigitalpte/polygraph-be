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
		ExamTypeID: &examType.ID,
		Category:   "relevant",
		Text:       "Did you take the missing funds?",
		SortOrder:  1,
	})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.True(t, created.Active)

	_, err = s.CreateQuestionTemplate(QuestionTemplateInput{
		ExamTypeID: &examType.ID,
		Category:   "not-a-category",
		Text:       "Bad category",
	})
	assert.Error(t, err)

	// The exam type is an optional tag — an untagged template is valid and is
	// offered for every booking.
	untagged, err := s.CreateQuestionTemplate(QuestionTemplateInput{
		Category: "relevant",
		Text:     "Applies to any exam type",
	})
	require.NoError(t, err)
	assert.Nil(t, untagged.ExamTypeID)

	list, err := s.ListQuestionTemplates(&examType.ID, false)
	require.NoError(t, err)
	assert.Len(t, list, 2, "tagged plus untagged templates are both offered")

	otherType := ExamType{Name: "Pre-employment", Duration: 90, Active: true}
	require.NoError(t, db.Create(&otherType).Error)
	otherList, err := s.ListQuestionTemplates(&otherType.ID, false)
	require.NoError(t, err)
	assert.Len(t, otherList, 1, "only the untagged template applies to an unrelated exam type")

	updated, err := s.UpdateQuestionTemplate(created.ID, QuestionTemplateUpdate{
		Text: strPtr("Did you steal the missing funds?"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Did you steal the missing funds?", updated.Text)
	// A partial update must not disturb the fields it did not mention — reordering
	// a row must never deactivate it or drop its exam type.
	assert.Equal(t, "relevant", updated.Category)
	assert.True(t, updated.Active)
	require.NotNil(t, updated.ExamTypeID)
	assert.Equal(t, examType.ID, *updated.ExamTypeID)

	reordered, err := s.UpdateQuestionTemplate(created.ID, QuestionTemplateUpdate{SortOrder: intPtr(7)})
	require.NoError(t, err)
	assert.Equal(t, 7, reordered.SortOrder)
	assert.True(t, reordered.Active)
	require.NotNil(t, reordered.ExamTypeID)

	// An explicit 0 clears the tag — the picker's "Any exam type" option.
	cleared, err := s.UpdateQuestionTemplate(created.ID, QuestionTemplateUpdate{ExamTypeID: uintPtr(0)})
	require.NoError(t, err)
	assert.Nil(t, cleared.ExamTypeID)

	require.NoError(t, s.DeactivateQuestionTemplate(created.ID))
	activeOnly, err := s.ListQuestionTemplates(&examType.ID, false)
	require.NoError(t, err)
	assert.Len(t, activeOnly, 1, "only the still-active untagged template remains")

	withInactive, err := s.ListQuestionTemplates(&examType.ID, true)
	require.NoError(t, err)
	assert.Len(t, withInactive, 2)
}
