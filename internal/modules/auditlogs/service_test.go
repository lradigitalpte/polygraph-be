package auditlogs

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"my-app/internal/models"
)

// setupTestDB creates an isolated in-memory SQLite database for each test.
// It migrates audit_logs and creates a minimal users table to satisfy the LEFT JOIN.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := fmt.Sprintf("file:auditdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.AuditLog{})
	require.NoError(t, err)

	// Minimal users table to satisfy the LEFT JOIN in service queries
	err = db.Exec(`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, email TEXT)`).Error
	require.NoError(t, err)

	return db
}

// seedLog inserts a single AuditLog row and returns it for assertions.
func seedLog(t *testing.T, db *gorm.DB, action, method, path string, status int) models.AuditLog {
	t.Helper()
	log := models.AuditLog{
		Action:    action,
		Method:    method,
		Path:      path,
		Status:    status,
		IP:        "127.0.0.1",
		UserAgent: "test-agent",
		Payload:   `{}`,
	}
	require.NoError(t, db.Create(&log).Error)
	return log
}

// TestService_GetAll_Empty verifies that GetAll returns an empty slice (not an error) when no logs exist.
func TestService_GetAll_Empty(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	rows, err := s.GetAll(100)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// TestService_GetAll_ReturnsLogs verifies that inserted logs are returned with correct field mapping.
func TestService_GetAll_ReturnsLogs(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	seedLog(t, db, "GET /api/leads", "GET", "/api/leads", 200)
	seedLog(t, db, "POST /api/leads", "POST", "/api/leads", 201)

	rows, err := s.GetAll(100)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

// TestService_GetAll_OrderedByCreatedAtDesc verifies the most recent log appears first.
func TestService_GetAll_OrderedByCreatedAtDesc(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	first := seedLog(t, db, "GET /api/leads", "GET", "/api/leads", 200)
	// Small sleep to ensure different timestamps
	time.Sleep(2 * time.Millisecond)
	second := seedLog(t, db, "POST /api/leads", "POST", "/api/leads", 201)

	rows, err := s.GetAll(100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	// Most recent (second) should be first
	assert.Equal(t, second.ID, rows[0].ID)
	assert.Equal(t, first.ID, rows[1].ID)
}

// TestService_GetAll_LimitIsRespected verifies that only `limit` rows are returned even when more exist.
func TestService_GetAll_LimitIsRespected(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	for i := 0; i < 10; i++ {
		seedLog(t, db, fmt.Sprintf("action-%d", i), "GET", "/api/test", 200)
	}

	rows, err := s.GetAll(3)
	require.NoError(t, err)
	assert.Len(t, rows, 3)
}

// TestService_GetByID_Found verifies that a known ID returns the correct log row.
func TestService_GetByID_Found(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	inserted := seedLog(t, db, "DELETE /api/leads/1", "DELETE", "/api/leads/1", 204)

	row, err := s.GetByID(inserted.ID)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, inserted.ID, row.ID)
	assert.Equal(t, "DELETE /api/leads/1", row.Action)
	assert.Equal(t, "DELETE", row.Method)
	assert.Equal(t, "/api/leads/1", row.Path)
	assert.Equal(t, 204, row.Status)
	assert.Equal(t, "127.0.0.1", row.IP)
}

// TestService_GetByID_NotFound verifies that a missing ID returns an error.
func TestService_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	row, err := s.GetByID(99999)
	assert.Error(t, err)
	assert.Nil(t, row)
}

// TestService_GetByID_UserEmailJoined verifies that user email is populated when a matching user exists.
func TestService_GetByID_UserEmailJoined(t *testing.T) {
	db := setupTestDB(t)
	s := &Service{db: db}

	// Insert a user
	require.NoError(t, db.Exec(`INSERT INTO users (id, email) VALUES (1, 'audit@example.com')`).Error)

	// Insert a log associated with that user
	userID := uint(1)
	log := models.AuditLog{
		UserID:    &userID,
		Action:    "GET /api/leads",
		Method:    "GET",
		Path:      "/api/leads",
		Status:    200,
		IP:        "10.0.0.1",
		UserAgent: "Go-Test/1.0",
		Payload:   `{}`,
	}
	require.NoError(t, db.Create(&log).Error)

	row, err := s.GetByID(log.ID)
	require.NoError(t, err)
	require.NotNil(t, row)
	require.NotNil(t, row.UserEmail)
	assert.Equal(t, "audit@example.com", *row.UserEmail)
	assert.Equal(t, &userID, row.UserID)
}
