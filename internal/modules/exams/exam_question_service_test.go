package exams

import (
	"strconv"
	"testing"
	"time"

	"my-app/internal/modules/subjects"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uintToString(v uint) string { return strconv.FormatUint(uint64(v), 10) }

// bookingFixture creates a client, subject, exam type and appointment ready for
// question prep.
func bookingFixture(t *testing.T, s *Service) (client testClient, subject subjects.Subject, examType ExamType, appt appointmentLink) {
	t.Helper()

	client = testClient{Name: "Acme Corp"}
	require.NoError(t, s.db.Create(&client).Error)

	subject = subjects.Subject{FirstName: "Jane", LastName: "Doe", IDNumber: "123456789"}
	require.NoError(t, s.db.Create(&subject).Error)

	examType = ExamType{Name: "Specific Issue", Duration: 150, Active: true}
	require.NoError(t, s.db.Create(&examType).Error)

	appt = appointmentLink{
		ClientID:    client.ID,
		SubjectID:   subject.ID,
		ExaminerID:  1,
		ScheduledAt: time.Date(2026, 9, 7, 13, 0, 0, 0, time.UTC),
		ExamTypeID:  &examType.ID,
		Status:      "pending",
	}
	require.NoError(t, s.db.Create(&appt).Error)
	return client, subject, examType, appt
}

func TestService_AppointmentQuestionsCopyToExamOnStart(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})
	_, _, examType, appt := bookingFixture(t, s)

	relevant, err := s.CreateQuestionTemplate(QuestionTemplateInput{
		ExamTypeID: &examType.ID, Category: "relevant", Text: "Did {{subject_name}} take the funds?", SortOrder: 1,
	})
	require.NoError(t, err)
	comparison, err := s.CreateQuestionTemplate(QuestionTemplateInput{
		Category: "comparison", Text: "Before today, have you ever lied to {{client_name}}?", SortOrder: 2,
	})
	require.NoError(t, err)
	assert.Nil(t, comparison.ExamTypeID, "untagged templates are allowed")

	added, err := s.AddAppointmentQuestionsFromTemplates(appt.ID, []uint{relevant.ID, comparison.ID})
	require.NoError(t, err)
	require.Len(t, added, 2)
	assert.Equal(t, "Did Jane Doe take the funds?", added[0].Text)
	assert.Contains(t, added[1].Text, "Acme Corp")

	// questions_prepared flips on so the booking UI can badge it.
	var flagged appointmentLink
	require.NoError(t, db.First(&flagged, appt.ID).Error)
	assert.True(t, flagged.QuestionsPrepared)

	exam, err := s.StartDocumentationForAppointment(uintToString(appt.ID))
	require.NoError(t, err)
	require.NotNil(t, exam)

	// Exam type carried over from the booking (previously always NULL).
	require.NotNil(t, exam.ExamTypeID)
	assert.Equal(t, examType.ID, *exam.ExamTypeID)
	assert.Equal(t, "Specific Issue", exam.Type)

	copied, err := s.ListExamQuestions(exam.ID)
	require.NoError(t, err)
	require.Len(t, copied, 2)
	assert.Equal(t, "Did Jane Doe take the funds?", copied[0].Text)
	assert.Equal(t, "relevant", copied[0].Category)
	assert.Equal(t, "comparison", copied[1].Category)
	assert.Equal(t, 0, copied[0].SortOrder)
	assert.Equal(t, 1, copied[1].SortOrder)
}

func TestService_AppointmentQuestionsFrozenAfterDocumentationStarts(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})
	_, _, _, appt := bookingFixture(t, s)

	_, err := s.ReplaceAppointmentQuestions(appt.ID, []AppointmentQuestionInput{
		{Text: "Prepared question", Category: "relevant"},
	})
	require.NoError(t, err)

	_, err = s.StartDocumentationForAppointment(uintToString(appt.ID))
	require.NoError(t, err)

	_, err = s.ReplaceAppointmentQuestions(appt.ID, []AppointmentQuestionInput{{Text: "Too late", Category: "relevant"}})
	assert.ErrorIs(t, err, errDocumentationStarted)

	_, err = s.AddAppointmentQuestionsFromTemplates(appt.ID, []uint{1})
	assert.ErrorIs(t, err, errDocumentationStarted)

	// The frozen rows are still readable as history.
	history, err := s.ListAppointmentQuestions(appt.ID)
	require.NoError(t, err)
	assert.Len(t, history, 1)
}

func TestService_EditingTemplateDoesNotAlterPreparedQuestions(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})
	_, _, examType, appt := bookingFixture(t, s)

	tpl, err := s.CreateQuestionTemplate(QuestionTemplateInput{
		ExamTypeID: &examType.ID, Category: "relevant", Text: "Original wording?",
	})
	require.NoError(t, err)

	_, err = s.AddAppointmentQuestionsFromTemplates(appt.ID, []uint{tpl.ID})
	require.NoError(t, err)

	_, err = s.UpdateQuestionTemplate(tpl.ID, QuestionTemplateUpdate{Text: strPtr("Completely rewritten?")})
	require.NoError(t, err)

	prepared, err := s.ListAppointmentQuestions(appt.ID)
	require.NoError(t, err)
	require.Len(t, prepared, 1)
	assert.Equal(t, "Original wording?", prepared[0].Text, "library edits must never rewrite a booking's snapshot")

	exam, err := s.StartDocumentationForAppointment(uintToString(appt.ID))
	require.NoError(t, err)
	_, err = s.UpdateQuestionTemplate(tpl.ID, QuestionTemplateUpdate{Text: strPtr("Rewritten again?")})
	require.NoError(t, err)

	copied, err := s.ListExamQuestions(exam.ID)
	require.NoError(t, err)
	require.Len(t, copied, 1)
	assert.Equal(t, "Original wording?", copied[0].Text)
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
func intPtr(v int) *int       { return &v }
func uintPtr(v uint) *uint    { return &v }
