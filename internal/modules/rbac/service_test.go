package rbac

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

	err = db.AutoMigrate(
		&Permission{},
		&Role{},
	)
	require.NoError(t, err)

	return db
}

func TestService_GetAllPermissions(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	db.Create(&Permission{Name: "exam:read", Group: "Exams"})
	db.Create(&Permission{Name: "exam:create", Group: "Exams"})

	permissions, err := s.GetAllPermissions()
	assert.NoError(t, err)
	assert.Len(t, permissions, 2)
}

func TestService_CreateAndGetRoles(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	perm1 := Permission{Name: "exam:read"}
	perm2 := Permission{Name: "exam:write"}
	db.Create(&perm1)
	db.Create(&perm2)

	role, err := s.CreateRole("Examiner", "A forensic examiner", []uint{perm1.ID, perm2.ID})
	assert.NoError(t, err)
	assert.NotZero(t, role.ID)
	assert.Equal(t, "Examiner", role.Name)

	roles, err := s.GetAllRoles()
	assert.NoError(t, err)
	assert.Len(t, roles, 1)
	assert.Len(t, roles[0].Permissions, 2)
	assert.Equal(t, "exam:read", roles[0].Permissions[0].Name)
}
