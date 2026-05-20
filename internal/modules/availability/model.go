package availability

import (
	"time"

	"gorm.io/gorm"
)

// Block represents a full-day or partial-day examiner availability override.
type Block struct {
	ID         uint           `gorm:"primarykey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	ExaminerID uint           `gorm:"index;not null" json:"examiner_id"`
	Date       time.Time      `gorm:"index;not null" json:"date"`
	StartTime  string         `gorm:"size:5" json:"start_time,omitempty"`
	EndTime    string         `gorm:"size:5" json:"end_time,omitempty"`
	IsFullDay  bool           `gorm:"default:false" json:"is_full_day"`
	Reason     string         `gorm:"type:text" json:"reason"`
}
