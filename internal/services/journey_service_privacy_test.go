package services

import (
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTripBody() *models.Journey {
	return &models.Journey{
		Title: "T", Destination: "Nagoya", Country: "Japan",
		DepartureDate: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		ReturnDate:    time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC),
	}
}

func TestCreateDefaultsPrivacy(t *testing.T) {
	e := newItinEnv(t)
	j := newTripBody()
	require.NoError(t, e.journeys.Create(e.ctx, "alice", j))
	got := e.reload(j)
	assert.Equal(t, models.VisibilityFollowers, got.Visibility)
	assert.Equal(t, "Asia/Bangkok", got.Timezone)
}

func TestCreateKeepsValidPrivacy(t *testing.T) {
	e := newItinEnv(t)
	j := newTripBody()
	j.Visibility, j.Timezone = models.VisibilityPrivate, "Asia/Tokyo"
	require.NoError(t, e.journeys.Create(e.ctx, "alice", j))
	got := e.reload(j)
	assert.Equal(t, models.VisibilityPrivate, got.Visibility)
	assert.Equal(t, "Asia/Tokyo", got.Timezone)
}

func TestCreateRejectsBadPrivacy(t *testing.T) {
	e := newItinEnv(t)
	for name, mut := range map[string]func(*models.Journey){
		"visibility": func(j *models.Journey) { j.Visibility = "secret" },
		"timezone":   func(j *models.Journey) { j.Timezone = "Nope/Zone" },
		"local":      func(j *models.Journey) { j.Timezone = "Local" },
		"category": func(j *models.Journey) {
			j.ItineraryDays = []models.ItineraryDay{{Date: "d", DateISO: j.DepartureDate, Plans: []models.ActivityPlan{{Description: "x", Category: "bogus"}}}}
		},
	} {
		j := newTripBody()
		mut(j)
		var ve *ValidationError
		assert.ErrorAs(t, e.journeys.Create(e.ctx, "alice", j), &ve, name)
	}
}

func TestCreateAutoHidesPlans(t *testing.T) {
	e := newItinEnv(t)
	j := newTripBody()
	j.ItineraryDays = []models.ItineraryDay{{Date: "d", DateISO: j.DepartureDate, Plans: []models.ActivityPlan{
		{Description: "hotel", Emoji: "🏨"},
		{Description: "stay", Category: "stay"},
		{Description: "food", Category: "food"},
		{Description: "explicit", Category: "food", HiddenFromPublic: true},
	}}}
	require.NoError(t, e.journeys.Create(e.ctx, "alice", j))
	byDesc := map[string]bool{}
	for _, p := range e.reload(j).ItineraryDays[0].Plans {
		byDesc[p.Description] = p.HiddenFromPublic
	}
	assert.Equal(t, map[string]bool{"hotel": true, "stay": true, "food": false, "explicit": true}, byDesc)
}

func TestFullPutLeavesPrivacyFieldsAndAutoHides(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	require.NoError(t, e.db.Model(j).Updates(map[string]any{"visibility": "link", "is_draft": true, "timezone": "Asia/Tokyo", "hide_live_status": true}).Error)
	body := newTripBody()
	body.Visibility, body.Timezone, body.IsDraft, body.HideLiveStatus = models.VisibilityPublic, "UTC", false, false
	body.ItineraryDays = []models.ItineraryDay{{Date: "d", DateISO: body.DepartureDate, Plans: []models.ActivityPlan{{Description: "hotel", Emoji: "🏨"}}}}
	require.NoError(t, e.journeys.Update(e.ctx, "alice", j.Slug, body))
	got := e.reload(j)
	assert.Equal(t, "link", got.Visibility)
	assert.True(t, got.IsDraft)
	assert.Equal(t, "Asia/Tokyo", got.Timezone)
	assert.True(t, got.HideLiveStatus)
	assert.True(t, got.ItineraryDays[0].Plans[0].HiddenFromPublic)
}
