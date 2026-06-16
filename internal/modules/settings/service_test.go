package settings

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-app/internal/models"
	"my-app/internal/modules/appointments"
	"my-app/internal/modules/auth"
	"my-app/internal/modules/availability"
	"my-app/internal/modules/exams"
	"my-app/internal/modules/forms"
	"my-app/internal/modules/intake"
	"my-app/internal/modules/leads"
	"my-app/internal/modules/rbac"
	"my-app/internal/modules/subjects"
)

func setupDeleteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")

	dbName := fmt.Sprintf("file:settingsdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&auth.User{},
		&models.AuditLog{},
		&rbac.Permission{},
		&rbac.Role{},
		&subjects.Subject{},
		&appointments.Client{},
		&appointments.Appointment{},
		&appointments.ClientDocument{},
		&appointments.SubjectDocument{},
		&appointments.DocumentShare{},
		&appointments.Quotation{},
		&availability.Block{},
		&leads.Lead{},
		&intake.IntakeRequest{},
		&exams.ExamType{},
		&exams.Exam{},
		&exams.ExamQuestion{},
		&exams.ExamReport{},
		&exams.Document{},
		&exams.CaseReferral{},
		&exams.ClinicalAssessment{},
		&exams.ExamPhase{},
		&forms.FormTemplate{},
		&forms.FormRequest{},
		&OrganizationSettings{},
	)
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys=ON").Error)

	forms.SeedTemplates(db)

	role := rbac.Role{Name: "Examiner"}
	require.NoError(t, db.Create(&role).Error)
	examiner := auth.User{Name: "Examiner", Email: "examiner@test.com", RoleID: role.ID, Status: "active"}
	require.NoError(t, db.Create(&examiner).Error)

	client := appointments.Client{Name: "Polygraphuae", Email: "org@test.com", ClientType: "Corporate"}
	require.NoError(t, db.Create(&client).Error)

	subject := subjects.Subject{ClientID: &client.ID, FirstName: "Prince", LastName: "Walker", Email: "prince@test.com"}
	require.NoError(t, db.Create(&subject).Error)

	scheduledAt := time.Now().Add(24 * time.Hour)
	appt := appointments.Appointment{
		ClientID: client.ID, SubjectID: subject.ID, ExaminerID: examiner.ID,
		ScheduledAt: scheduledAt, Duration: 60, Status: "pending",
	}
	require.NoError(t, db.Create(&appt).Error)

	exam := exams.Exam{
		ClientID: client.ID, SubjectID: subject.ID, ExaminerID: examiner.ID,
		AppointmentID: &appt.ID, Date: scheduledAt, Status: "scheduled",
	}
	require.NoError(t, db.Create(&exam).Error)

	appt.ExamID = &exam.ID
	require.NoError(t, db.Save(&appt).Error)

	var tpl forms.FormTemplate
	require.NoError(t, db.First(&tpl).Error)
	require.NoError(t, db.Create(&forms.FormRequest{
		Token: "token-abc", TemplateID: tpl.ID, ClientID: client.ID,
		RecipientEmail: "prince@test.com", SentAt: time.Now(), ExpiresAt: time.Now().Add(48 * time.Hour),
	}).Error)

	require.NoError(t, db.Create(&OrganizationSettings{
		ID: singletonID, Name: "Polygraphuae",
	}).Error)

	return db
}

func TestDeleteOrganizationData_WipesOperationalDataAndReseedsTemplates(t *testing.T) {
	db := setupDeleteTestDB(t)
	svc := &Service{db: db}

	require.NoError(t, svc.DeleteOrganizationData())

	var clientCount int64
	require.NoError(t, db.Model(&appointments.Client{}).Count(&clientCount).Error)
	assert.Equal(t, int64(0), clientCount)

	var userCount int64
	require.NoError(t, db.Model(&auth.User{}).Count(&userCount).Error)
	assert.Equal(t, int64(1), userCount)

	var templateCount int64
	require.NoError(t, db.Unscoped().Model(&forms.FormTemplate{}).Count(&templateCount).Error)
	assert.Greater(t, templateCount, int64(0))

	var orgCount int64
	require.NoError(t, db.Model(&OrganizationSettings{}).Count(&orgCount).Error)
	assert.Equal(t, int64(0), orgCount)
}
