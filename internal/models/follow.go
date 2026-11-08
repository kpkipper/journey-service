package models

import "time"

// Follow records that FollowerID follows FolloweeID. No approval step.
type Follow struct {
	FollowerID string    `gorm:"type:varchar(10);primaryKey" json:"-"`
	FolloweeID string    `gorm:"type:varchar(10);primaryKey;index" json:"-"`
	CreatedAt  time.Time `json:"created_at"`
}
