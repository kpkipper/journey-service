package models

import "time"

// Block records that BlockerID blocked BlockedID. The row is one-directional
// but its effect applies both ways.
type Block struct {
	BlockerID string    `gorm:"type:varchar(10);primaryKey" json:"-"`
	BlockedID string    `gorm:"type:varchar(10);primaryKey;index" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}
