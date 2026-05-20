package auth

import (
	"my-app/internal/modules/rbac"
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID                    uint           `gorm:"primarykey" json:"id"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
	Name                  string         `gorm:"size:255;not null" json:"name"`
	Email                 string         `gorm:"size:255;uniqueIndex;not null" json:"email"`
	Status                string         `gorm:"size:50;default:'active';not null" json:"status"`
	InvitedAt             *time.Time     `json:"invited_at,omitempty"`
	LastActiveAt          *time.Time     `json:"last_active_at,omitempty"`
	SuspendedAt           *time.Time     `json:"suspended_at,omitempty"`
	PasswordResetRequired bool           `gorm:"default:false" json:"password_reset_required"`
	PasswordResetSentAt   *time.Time     `json:"password_reset_sent_at,omitempty"`
	RoleID                uint           `json:"role_id"`
	Role                  rbac.Role      `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}
