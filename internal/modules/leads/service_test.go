package leads

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&Lead{})
	require.NoError(t, err)

	return db
}

func TestService_Create_DefaultsAndRef(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	created, err := s.Create(&CreateLeadInput{
		Name:     "Jane Doe",
		Email:    "jane@example.com",
		Phone:    "0900000000",
		Source:   "Website",
		Interest: "Premium package",
	})

	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotZero(t, created.ID)
	assert.Equal(t, StatusNew, created.Status)
	assert.Equal(t, PriorityStandard, created.Priority)
	assert.Equal(t, "LD-0001", created.Ref)
}

func TestService_GetAll_OrdersByCreatedDesc(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	first, err := s.Create(&CreateLeadInput{
		Name:     "First Lead",
		Email:    "first@example.com",
		Source:   "Facebook",
		Interest: "Basic",
	})
	require.NoError(t, err)

	second, err := s.Create(&CreateLeadInput{
		Name:     "Second Lead",
		Email:    "second@example.com",
		Source:   "Referral",
		Interest: "Standard",
	})
	require.NoError(t, err)

	all, err := s.GetAll()
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, second.ID, all[0].ID)
	assert.Equal(t, first.ID, all[1].ID)
}

func TestService_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	lead, err := s.GetByID(99999)
	assert.Error(t, err)
	assert.Nil(t, lead)
}

func TestService_Update_PartialFields(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	created, err := s.Create(&CreateLeadInput{
		Name:     "Update Lead",
		Email:    "update@example.com",
		Source:   "Website",
		Interest: "Package",
		Notes:    "Initial note",
	})
	require.NoError(t, err)

	newStatus := StatusQualified
	newPriority := PriorityHigh
	newValue := 1200.50

	updated, err := s.Update(created.ID, &UpdateLeadInput{
		Status:         &newStatus,
		Priority:       &newPriority,
		EstimatedValue: &newValue,
		Notes:          "Updated notes",
		NextStep:       "Call tomorrow",
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, StatusQualified, updated.Status)
	assert.Equal(t, PriorityHigh, updated.Priority)
	assert.Equal(t, 1200.50, updated.EstimatedValue)
	assert.Equal(t, "Updated notes", updated.Notes)
	assert.Equal(t, "Call tomorrow", updated.NextStep)
}

func TestService_Delete_SoftDelete(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	created, err := s.Create(&CreateLeadInput{
		Name:     "Delete Lead",
		Email:    "delete@example.com",
		Source:   "Instagram",
		Interest: "Consultation",
	})
	require.NoError(t, err)

	err = s.Delete(created.ID)
	require.NoError(t, err)

	_, err = s.GetByID(created.ID)
	assert.Error(t, err)

	var deleted Lead
	err = db.Unscoped().First(&deleted, created.ID).Error
	require.NoError(t, err)
	assert.True(t, deleted.DeletedAt.Valid)
}
