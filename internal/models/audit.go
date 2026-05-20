package models

import (
	"time"
)

// AuditLog records every system action for forensic compliance
type AuditLog struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UserID    *uint     `gorm:"index" json:"user_id"`
	Action    string    `json:"action"`
	Method    string    `json:"method"`
	Path      string    `gorm:"index" json:"path"`
	Status    int       `gorm:"index" json:"status"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Payload   string    `gorm:"column:payload;type:text" json:"payload"`
}
