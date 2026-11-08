package models

import (
	"fmt"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"gorm.io/gorm"
)

const nanoidAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
const nanoidLen = 10

func newShortID() string {
	id, _ := gonanoid.Generate(nanoidAlphabet, nanoidLen)
	return id
}

const (
	VisibilityPublic    = "public"
	VisibilityFollowers = "followers"
	VisibilityLink      = "link"
	VisibilityPrivate   = "private"
)

type Journey struct {
	ID               string         `gorm:"type:varchar(10);primaryKey" json:"id"`
	Slug             string         `gorm:"type:varchar(100);uniqueIndex" json:"slug"`
	OwnerID          string         `gorm:"type:varchar(10);index" json:"owner_id"`
	Title            string         `gorm:"not null" json:"title"`
	Destination      string         `gorm:"not null" json:"destination"`
	Country          string         `json:"country"`
	DepartureDate    time.Time      `gorm:"not null" json:"departure_date"`
	ReturnDate       time.Time      `gorm:"not null" json:"return_date"`
	CoverURL         string         `json:"cover_url"`
	EditorsCanInvite bool           `gorm:"not null;default:false" json:"editors_can_invite"`
	Visibility       string         `gorm:"type:varchar(10);not null;default:'followers'" json:"visibility"`
	IsDraft          bool           `gorm:"not null;default:false" json:"is_draft"`
	Timezone         string         `gorm:"type:varchar(64);not null;default:'Asia/Bangkok'" json:"timezone"`
	HideLiveStatus   bool           `gorm:"not null;default:false" json:"hide_live_status"`
	ItineraryDays    []ItineraryDay `gorm:"foreignKey:JourneyID;constraint:OnDelete:CASCADE" json:"itinerary_days,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func toSlugPart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ",", "-")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func (j *Journey) BeforeCreate(tx *gorm.DB) error {
	if j.ID == "" {
		j.ID = newShortID()
	}
	j.Slug = fmt.Sprintf("%s-%s", toSlugPart(j.Destination), j.ID)
	return nil
}

type ItineraryDay struct {
	ID        string         `gorm:"type:varchar(10);primaryKey" json:"id"`
	JourneyID string         `gorm:"type:varchar(10);not null;index" json:"-"`
	Date      string         `gorm:"not null" json:"date"`
	DateISO   time.Time      `gorm:"not null" json:"date_iso"`
	Title     string         `json:"title"`
	Plans     []ActivityPlan `gorm:"foreignKey:ItineraryDayID;constraint:OnDelete:CASCADE" json:"plans,omitempty"`
}

func (d *ItineraryDay) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = newShortID()
	}
	return nil
}

type ActivityPlan struct {
	ID             string `gorm:"type:varchar(10);primaryKey" json:"id"`
	ItineraryDayID string `gorm:"type:varchar(10);not null;index" json:"day_id"`
	Time           string `json:"time"`
	Description    string `gorm:"not null" json:"description"`
	Emoji          string `json:"emoji"`
	MapURL         string `json:"map_url"`
	Position       int    `gorm:"not null;default:0" json:"position"`
	// Category is one of stay, food, sight, transport, other; may be empty.
	Category         string `gorm:"type:varchar(20)" json:"category"`
	HiddenFromPublic bool   `gorm:"not null;default:false" json:"hidden_from_public"`
}

func (p *ActivityPlan) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = newShortID()
	}
	return nil
}

type JourneyListItem struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Destination string `json:"destination"`
}

type JourneyByCountry struct {
	Country string            `json:"country"`
	Plan    []JourneyListItem `json:"plan"`
}
