package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	MomentPhoto   = "photo"
	MomentVideo   = "video"
	MomentPending = "pending"
	MomentReady   = "ready"
	// MomentRejected keeps the row of a refused upload so it still counts
	// towards the per-user start rate limit; it is swept later.
	MomentRejected = "rejected"
)

// Moment is a photo or video attached to a trip day (and optionally a plan).
// It has no soft delete.
type Moment struct {
	ID          string `gorm:"type:varchar(10);primaryKey" json:"id"`
	JourneyID   string `gorm:"type:varchar(10);not null;index" json:"journey_id"`
	DayID       string `gorm:"type:varchar(10);not null;index" json:"day_id"`
	PlanID      string `gorm:"type:varchar(10);index" json:"plan_id"`
	UploaderID  string `gorm:"type:varchar(10);not null;index" json:"uploader_id"`
	Kind        string `gorm:"type:varchar(10)" json:"kind"`
	Status      string `gorm:"type:varchar(10);index" json:"status"`
	Caption     string `gorm:"type:varchar(1200)" json:"caption"`
	ContentType string `gorm:"type:varchar(50)" json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	DurationMs  int64  `json:"duration_ms"`
	// MembersOnly keeps a moment of a deleted hidden_from_public plan (e.g. the
	// hotel) hidden from non-members after it falls back to day level.
	MembersOnly bool      `gorm:"not null;default:false" json:"-"`
	ObjectKey   string    `gorm:"type:varchar(200)" json:"-"`
	ThumbKey    string    `gorm:"type:varchar(200)" json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (m *Moment) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = newShortID()
	}
	return nil
}
