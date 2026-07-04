package exams

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"my-app/internal/models"
	"my-app/internal/modules/subjects"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Set an encryption key for tests since subjects and reports require it
	os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Cleanup(func() {
		os.Unsetenv("ENCRYPTION_KEY")
	})

	dbName := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&subjects.Subject{},
		&Exam{},
		&ExamQuestion{},
		&ExamReport{},
		&Document{},
		&CaseReferral{},
		&ClinicalAssessment{},
		&ExamPhase{},
		&models.AuditLog{},
	)
	require.NoError(t, err)

	return db
}

// MockStorage implements storage.Storage
type MockStorage struct {
	UploadFunc func(ctx context.Context, key string, body io.Reader, contentType string) (string, error)
}

func (m *MockStorage) UploadFile(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	if m.UploadFunc != nil {
		return m.UploadFunc(ctx, key, body, contentType)
	}
	return "mock-url/" + key, nil
}

func (m *MockStorage) DeleteFile(ctx context.Context, key string) error {
	return nil
}

func (m *MockStorage) GetSignedURL(ctx context.Context, key string) (string, error) {
	return "signed-" + key, nil
}

func TestService_CreateAndGetAllExams(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})

	subject := subjects.Subject{
		FirstName: "John",
		LastName:  "Doe",
		IDNumber:  "123456789",
	}
	require.NoError(t, db.Create(&subject).Error)

	exam := &Exam{
		ClientID:   1,
		SubjectID:  subject.ID,
		ExaminerID: 2,
		Date:       time.Now(),
		Type:       "Pre-employment",
	}

	err := s.CreateExam(exam)
	assert.NoError(t, err)
	assert.NotZero(t, exam.ID)

	exams, err := s.GetAllExams()
	assert.NoError(t, err)
	assert.Len(t, exams, 1)
	assert.Equal(t, subject.ID, exams[0].Subject.ID) // Verify preload
}

func TestService_UploadDocument(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})

	ctx := context.Background()
	body := strings.NewReader("test file content")

	doc, err := s.UploadDocument(ctx, 1, "test.txt", "document", body)
	assert.NoError(t, err)
	assert.NotNil(t, doc)

	assert.Equal(t, uint(1), doc.ExamID)
	assert.Equal(t, "test.txt", doc.Name)
	assert.Equal(t, "document", doc.Type)
	assert.Contains(t, doc.URL, "mock-url/exams/1/test.txt")
	assert.NotEmpty(t, doc.Hash) // SHA256 was computed
}

func TestService_FinalizeReport(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})

	exam := &Exam{ClientID: 1, SubjectID: 1, ExaminerID: 1}
	require.NoError(t, s.CreateExam(exam))

	_, err := s.CreateReport(exam.ID, "NDI", "Subject is telling the truth")
	require.NoError(t, err)

	finalized, err := s.FinalizeReport(exam.ID, 1, "examiner@example.com")
	assert.NoError(t, err)
	assert.True(t, finalized.IsLocked)
	assert.NotNil(t, finalized.LockedAt)
	assert.NotEmpty(t, finalized.SignatureExaminer)

	_, err = s.FinalizeReport(exam.ID, 1, "examiner@example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already locked")
}

func TestService_CreateSecureShareRequiresLock(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})

	exam := &Exam{ClientID: 1, SubjectID: 1, ExaminerID: 1}
	require.NoError(t, s.CreateExam(exam))

	report, err := s.CreateReport(exam.ID, "NDI", "Draft content")
	require.NoError(t, err)

	_, err = s.CreateSecureShare(report.ID, "client@example.com", 7)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "finalized and locked")
}

func TestService_CreateAndGetReport(t *testing.T) {
	db := setupTestDB(t)
	s := NewService(db, &MockStorage{})

	exam := &Exam{ClientID: 1, SubjectID: 1, ExaminerID: 1}
	require.NoError(t, s.CreateExam(exam))

	report, err := s.CreateReport(exam.ID, "NDI", "Subject is telling the truth")
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, "NDI", report.Verdict)
	assert.NotEmpty(t, report.EncryptedReport)

	// Fetch it back
	fetchedReport, decryptedStr, err := s.GetReport(exam.ID)
	assert.NoError(t, err)
	assert.Equal(t, report.ID, fetchedReport.ID)
	assert.Equal(t, report.EncryptedReport, fetchedReport.EncryptedReport)
	assert.Equal(t, "Subject is telling the truth", decryptedStr)
	assert.NotEmpty(t, fetchedReport.Hash)

	// Test Signing, which locks the report
	err = s.SignAndLockReport(exam.ID, "examiner-base64-sig", "examiner")
	assert.NoError(t, err)

	// Try to update after locking - Should fail!
	err = db.Model(&report).Update("verdict", "DI").Error
	assert.Error(t, err)
	if err != nil {
		assert.Contains(t, err.Error(), "cannot modify a locked forensic report")
	}
}
