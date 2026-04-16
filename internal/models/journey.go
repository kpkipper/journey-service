package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Journey struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Title         string         `gorm:"not null" json:"title"`
	Destination   string         `gorm:"not null" json:"destination"`
	Country       string         `json:"country"`
	DepartureDate time.Time      `gorm:"not null" json:"departure_date"`
	ReturnDate    time.Time      `gorm:"not null" json:"return_date"`
	ItineraryDays []ItineraryDay `gorm:"foreignKey:JourneyID;constraint:OnDelete:CASCADE" json:"itinerary_days,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (j *Journey) BeforeCreate(tx *gorm.DB) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return nil
}

type ItineraryDay struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	JourneyID uuid.UUID `gorm:"type:uuid;not null;index" json:"journey_id"`
	Date      string    `gorm:"not null" json:"date"`
	DateISO   time.Time `gorm:"not null" json:"date_iso"`
	Title     string    `json:"title"`
	Plans     []Plan    `gorm:"foreignKey:ItineraryDayID;constraint:OnDelete:CASCADE" json:"plans,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (d *ItineraryDay) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

type Plan struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ItineraryDayID uuid.UUID `gorm:"type:uuid;not null;index" json:"itinerary_day_id"`
	Time           string    `json:"time"`
	Description    string    `gorm:"not null" json:"description"`
	Country        string    `json:"country"`
	Emoji          string    `json:"emoji"`
	MapURL         string    `json:"map_url"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (p *Plan) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

type JourneyListItem struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Destination string    `json:"destination"`
}

type JourneyByCountry struct {
	Country string            `json:"country"`
	Plan    []JourneyListItem `json:"plan"`
}
