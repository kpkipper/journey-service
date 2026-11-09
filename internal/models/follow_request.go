package models

import "time"

// FollowRequest is a pending request by RequesterID to follow TargetID, who
// has follow approval on. Accepted followers live in follows, not here.
type FollowRequest struct {
	RequesterID string    `gorm:"type:varchar(10);primaryKey" json:"-"`
	TargetID    string    `gorm:"type:varchar(10);primaryKey;index" json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

// FollowEntry is a user in a follower or follow-request list.
type FollowEntry struct {
	User      PublicProfile `json:"user"`
	CreatedAt time.Time     `json:"created_at"`
}
