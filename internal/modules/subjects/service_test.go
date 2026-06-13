package subjects

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	// Set an encryption key for tests
	os.Setenv("ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Cleanup(func() {
		os.Unsetenv("ENCRYPTION_KEY")
	})

	t.Helper()
	dbName := fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&Subject{},
	)
	require.NoError(t, err)

	return db
}

func TestService_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	subj := &Subject{
		FirstName: "Jane",
		LastName:  "Doe",
		IDNumber:  "AAA123456",
	}

	err := s.Create(subj)
	assert.NoError(t, err)
	assert.NotZero(t, subj.ID)

	all, err := s.GetAll("", "")
	assert.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, "Jane", all[0].FirstName)
}

func TestService_CreateAllowsBlankIDNumber(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	first := &Subject{
		FirstName: "Test",
		LastName:  "Record",
	}
	second := &Subject{
		FirstName: "Another",
		LastName:  "Record",
	}

	err := s.Create(first)
	assert.NoError(t, err)

	err = s.Create(second)
	assert.NoError(t, err)

	all, err := s.GetAll("", "")
	assert.NoError(t, err)
	assert.Len(t, all, 2)
}
