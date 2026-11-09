package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	ReportTargetUser    = "user"
	ReportTargetJourney = "journey"
	ReportTargetMoment  = "moment"

	ReportOpen     = "open"
	ReportResolved = "resolved"
)

// Report is a user's complaint about a user, trip or moment. TargetID is the
// internal id (user id / journey id / moment id), never a username or slug.
type Report struct {
	ID         string     `gorm:"type:varchar(10);primaryKey" json:"id"`
	ReporterID string     `gorm:"type:varchar(10);not null;index" json:"reporter_id"`
	TargetType string     `gorm:"type:varchar(10);not null" json:"target_type"`
	TargetID   string     `gorm:"type:varchar(10);not null;index" json:"target_id"`
	Reason     string     `gorm:"type:varchar(20);not null" json:"reason"`
	Note       string     `gorm:"type:varchar(2000)" json:"note"`
	Status     string     `gorm:"type:varchar(10);not null;default:'open';index" json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

func (r *Report) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = newShortID()
	}
	return nil
}
