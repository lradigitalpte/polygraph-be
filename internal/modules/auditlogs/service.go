package auditlogs

import (
	"my-app/internal/database"
	"time"

	"gorm.io/gorm"
)

// Service handles audit log read operations.
type Service struct {
	db *gorm.DB
}

// NewService creates a Service backed by the shared database connection.
func NewService() *Service {
	return &Service{db: database.GetDB()}
}

// AuditLogRow represents a flattened audit log row enriched with user email.
type AuditLogRow struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    *uint     `json:"user_id"`
	UserEmail *string   `json:"user_email"`
	Action    string    `json:"action"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Payload   string    `json:"payload"`
}

// GetAll returns the most recent audit logs up to the provided limit.
func (s *Service) GetAll(limit int) ([]AuditLogRow, error) {
	var rows []AuditLogRow

	err := s.db.
		Table("audit_logs").
		Select(`audit_logs.id, audit_logs.created_at, audit_logs.user_id, users.email AS user_email, audit_logs.action, audit_logs.method, audit_logs.path, audit_logs.status, audit_logs.ip, audit_logs.user_agent, audit_logs.payload`).
		Joins("LEFT JOIN users ON users.id = audit_logs.user_id").
		Order("audit_logs.created_at DESC").
		Limit(limit).
		Find(&rows).Error

	if err != nil {
		return nil, err
	}

	return rows, nil
}

// GetByID returns a single audit log by ID.
func (s *Service) GetByID(id uint) (*AuditLogRow, error) {
	var row AuditLogRow

	err := s.db.
		Table("audit_logs").
		Select(`audit_logs.id, audit_logs.created_at, audit_logs.user_id, users.email AS user_email, audit_logs.action, audit_logs.method, audit_logs.path, audit_logs.status, audit_logs.ip, audit_logs.user_agent, audit_logs.payload`).
		Joins("LEFT JOIN users ON users.id = audit_logs.user_id").
		Where("audit_logs.id = ?", id).
		First(&row).Error

	if err != nil {
		return nil, err
	}

	return &row, nil
}
