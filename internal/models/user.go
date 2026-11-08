package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID          string    `gorm:"type:varchar(10);primaryKey" json:"id"`
	Email       string    `gorm:"uniqueIndex;not null" json:"email"`
	GoogleSub   *string   `gorm:"uniqueIndex" json:"-"`
	Username    string    `gorm:"type:varchar(20);uniqueIndex;not null" json:"username"`
	DisplayName string    `gorm:"type:varchar(50)" json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Bio         string    `gorm:"type:varchar(160)" json:"bio"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = newShortID()
	}
	return nil
}

type Session struct {
	ID        string    `gorm:"type:varchar(10);primaryKey"`
	UserID    string    `gorm:"type:varchar(10);not null;index"`
	TokenHash string    `gorm:"type:char(64);uniqueIndex;not null"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time
}

func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = newShortID()
	}
	return nil
}

type LoginToken struct {
	ID        string    `gorm:"type:varchar(10);primaryKey"`
	Email     string    `gorm:"not null;index"`
	TokenHash string    `gorm:"type:char(64);uniqueIndex;not null"`
	ExpiresAt time.Time `gorm:"not null"`
	UsedAt    *time.Time
	CreatedAt time.Time `gorm:"index"`
}

func (t *LoginToken) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = newShortID()
	}
	return nil
}

// ProfileUpdate: every field is a pointer; nil means "leave unchanged".
type ProfileUpdate struct {
	Username    *string `json:"username"`
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	AvatarURL   *string `json:"avatar_url"`
}

// PublicProfile is the user data safe to expose publicly (no email).
type PublicProfile struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Bio         string `json:"bio"`
}
