package leads

import (
	"time"

	"gorm.io/gorm"
)

// LeadStatus represents the stage of a lead in the pipeline.
type LeadStatus string

const (
	StatusNew       LeadStatus = "New"
	StatusContacted LeadStatus = "Contacted"
	StatusQualified LeadStatus = "Qualified"
	StatusConverted LeadStatus = "Converted"
	StatusLost      LeadStatus = "Lost"
)

// LeadPriority represents the urgency of a lead.
type LeadPriority string

const (
	PriorityLow      LeadPriority = "Low"
	PriorityStandard LeadPriority = "Standard"
	PriorityHigh     LeadPriority = "Priority"
	PriorityUrgent   LeadPriority = "Urgent"
)

// Lead represents a prospective client intake record.
type Lead struct {
	ID        uint           `gorm:"primarykey"           json:"id"`
	CreatedAt time.Time      `                            json:"created_at"`
	UpdatedAt time.Time      `                            json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                json:"-"`

	// Identity
	Name  string `gorm:"size:200;not null" json:"name"`
	Email string `gorm:"size:200"          json:"email"`
	Phone string `gorm:"size:50"           json:"phone"`

	// Acquisition metadata
	Source           string `gorm:"size:50;not null"  json:"source"`
	Interest         string `gorm:"size:200;not null" json:"interest"`
	Notes            string `gorm:"type:text"         json:"notes"`
	PreferredContact string `gorm:"size:20"           json:"preferred_contact"`

	// Pipeline state
	Status   LeadStatus   `gorm:"size:20;not null;default:'New'" json:"status"`
	Priority LeadPriority `gorm:"size:20;not null;default:'Standard'" json:"priority"`

	// Valuation
	EstimatedValue float64 `gorm:"default:0" json:"estimated_value"`

	// Next action description
	NextStep string `gorm:"size:500" json:"next_step"`

	// Unique intake reference (e.g. LD-5021)
	Ref string `gorm:"size:20;uniqueIndex" json:"ref"`
}

// CreateLeadInput is the payload accepted by the POST /leads endpoint.
type CreateLeadInput struct {
	Name             string       `json:"name"              binding:"required"`
	Email            string       `json:"email"             binding:"required,email"`
	Phone            string       `json:"phone"`
	Source           string       `json:"source"            binding:"required"`
	Interest         string       `json:"interest"          binding:"required"`
	Notes            string       `json:"notes"`
	PreferredContact string       `json:"preferred_contact"`
	Priority         LeadPriority `json:"priority"`
	EstimatedValue   float64      `json:"estimated_value"`
	NextStep         string       `json:"next_step"`
}

// UpdateLeadInput is the payload accepted by the PATCH /leads/:id endpoint.
type UpdateLeadInput struct {
	Status         *LeadStatus   `json:"status"`
	Priority       *LeadPriority `json:"priority"`
	Notes          string        `json:"notes"`
	NextStep       string        `json:"next_step"`
	EstimatedValue *float64      `json:"estimated_value"`
}
