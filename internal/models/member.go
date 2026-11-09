package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	RoleOwner    = "owner" // response only, never stored in the DB
	RoleEditor   = "editor"
	RoleTraveler = "traveler"

	StatusInvited = "invited"
	StatusJoined  = "joined"
)

type JourneyMember struct {
	ID        string    `gorm:"type:varchar(10);primaryKey" json:"id"`
	JourneyID string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_member_journey_user" json:"-"`
	UserID    string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_member_journey_user;index" json:"-"`
	Role      string    `gorm:"type:varchar(10);not null" json:"role"`
	Status    string    `gorm:"type:varchar(10);not null" json:"status"`
	InvitedBy string    `gorm:"type:varchar(10)" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (m *JourneyMember) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = newShortID()
	}
	return nil
}

type InviteLink struct {
	ID        string     `gorm:"type:varchar(10);primaryKey" json:"id"`
	JourneyID string     `gorm:"type:varchar(10);not null;index" json:"-"`
	TokenHash string     `gorm:"type:char(64);uniqueIndex;not null" json:"-"`
	Role      string     `gorm:"type:varchar(10);not null" json:"role"`
	CreatedBy string     `gorm:"type:varchar(10);not null" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (l *InviteLink) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = newShortID()
	}
	return nil
}

// MemberView is the member row exposed by the API.
type MemberView struct {
	UserID      string `json:"-"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}
