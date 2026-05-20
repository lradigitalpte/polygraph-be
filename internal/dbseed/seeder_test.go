package dbseed

import (
	"testing"

	"my-app/internal/modules/rbac"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestSeedDatabase_UserRoleHasLeadCRUDPermissions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:seeddb?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&rbac.Permission{}, &rbac.Role{})
	require.NoError(t, err)

	SeedDatabase(db, zap.NewNop())

	var userRole rbac.Role
	err = db.Preload("Permissions").Where("name = ?", "User").First(&userRole).Error
	require.NoError(t, err)

	permMap := map[string]bool{}
	for _, p := range userRole.Permissions {
		permMap[p.Name] = true
	}

	assert.True(t, permMap["lead:view"])
	assert.True(t, permMap["lead:create"])
	assert.True(t, permMap["lead:update"])
	assert.True(t, permMap["lead:delete"])
}
