package users

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-app/internal/models"
	"my-app/internal/modules/auth"
	"my-app/internal/modules/rbac"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := fmt.Sprintf("file:usersdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&rbac.Permission{}, &rbac.Role{}, &auth.User{}, &models.AuditLog{})
	require.NoError(t, err)

	return db
}

func seedRole(t *testing.T, db *gorm.DB, name string) rbac.Role {
	t.Helper()
	role := rbac.Role{Name: name}
	require.NoError(t, db.Create(&role).Error)
	return role
}

func TestService_CreateAndGetUser(t *testing.T) {
	db := setupTestDB(t)
	role := seedRole(t, db, "Admin")
	s := &Service{db: db}

	created, err := s.Create(CreateUserInput{
		Name:   "Admin User",
		Email:  "admin@example.com",
		RoleID: role.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, "pending", created.Status)
	assert.True(t, created.PasswordResetRequired)
	assert.Equal(t, role.ID, created.RoleID)

	fetched, err := s.GetByID(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin@example.com", fetched.Email)
	assert.Equal(t, "Admin", fetched.Role.Name)
}

func TestService_UpdateStatusAndRole(t *testing.T) {
	db := setupTestDB(t)
	roleA := seedRole(t, db, "Admin")
	roleB := seedRole(t, db, "Examiner")
	s := &Service{db: db}

	created, err := s.Create(CreateUserInput{Name: "Jane Doe", Email: "jane@example.com", RoleID: roleA.ID})
	require.NoError(t, err)

	updatedRole, err := s.UpdateRole(created.ID, roleB.ID)
	require.NoError(t, err)
	assert.Equal(t, roleB.ID, updatedRole.RoleID)
	assert.Equal(t, "Examiner", updatedRole.Role.Name)

	updatedStatus, err := s.UpdateStatus(created.ID, "suspended")
	require.NoError(t, err)
	assert.Equal(t, "suspended", updatedStatus.Status)
	assert.NotNil(t, updatedStatus.SuspendedAt)
}

func TestService_RequirePasswordResetAndActivity(t *testing.T) {
	db := setupTestDB(t)
	role := seedRole(t, db, "User")
	s := &Service{db: db}

	created, err := s.Create(CreateUserInput{Name: "John Doe", Email: "john@example.com", RoleID: role.ID})
	require.NoError(t, err)

	require.NoError(t, db.Create(&models.AuditLog{UserID: &created.ID, Action: "PATCH /api/users/1/status", Method: "PATCH", Path: "/api/users/1/status", Status: 200}).Error)
	require.NoError(t, db.Create(&models.AuditLog{UserID: &created.ID, Action: "POST /api/users/1/require-password-reset", Method: "POST", Path: "/api/users/1/require-password-reset", Status: 200}).Error)

	updated, err := s.RequirePasswordReset(created.ID)
	require.NoError(t, err)
	assert.True(t, updated.PasswordResetRequired)
	assert.NotNil(t, updated.PasswordResetSentAt)

	logs, err := s.GetActivity(created.ID, 10)
	require.NoError(t, err)
	assert.Len(t, logs, 2)
}
