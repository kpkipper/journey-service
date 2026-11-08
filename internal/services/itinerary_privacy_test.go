package services

import (
	"testing"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchPrivacy(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()

	_, err := e.svc.PatchJourney(e.ctx, "edtr", j.Slug, JourneyPatch{Visibility: sp("public")})
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.PatchJourney(e.ctx, "edtr", j.Slug, JourneyPatch{HideLiveStatus: bp(true)})
	assert.ErrorIs(t, err, ErrForbidden)

	got, err := e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{Visibility: sp("link"), HideLiveStatus: bp(true)})
	require.NoError(t, err)
	assert.Equal(t, "link", got.Visibility)
	assert.True(t, got.HideLiveStatus)

	var ve *ValidationError
	_, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{Visibility: sp("secret")})
	assert.ErrorAs(t, err, &ve)

	// editor may change draft flag and timezone; false persists
	got, err = e.svc.PatchJourney(e.ctx, "edtr", j.Slug, JourneyPatch{IsDraft: bp(true), Timezone: sp("Asia/Tokyo")})
	require.NoError(t, err)
	assert.True(t, got.IsDraft)
	assert.Equal(t, "Asia/Tokyo", got.Timezone)
	got, err = e.svc.PatchJourney(e.ctx, "edtr", j.Slug, JourneyPatch{IsDraft: bp(false)})
	require.NoError(t, err)
	assert.False(t, got.IsDraft)
	got, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{HideLiveStatus: bp(false)})
	require.NoError(t, err)
	assert.False(t, got.HideLiveStatus)

	for _, tz := range []string{"Nope/Zone", "Local", ""} {
		_, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{Timezone: sp(tz)})
		assert.ErrorAs(t, err, &ve, tz)
	}
	got, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{Timezone: sp("UTC")})
	require.NoError(t, err)
	assert.Equal(t, "UTC", got.Timezone)
}

func TestPlanHiddenFromPublic(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	d := e.day(j, "d1")
	add := func(in PlanInput) *models.ActivityPlan {
		in.Description = sp("x")
		p, err := e.svc.AddPlan(e.ctx, "alice", j.Slug, d.ID, in)
		require.NoError(t, err)
		return p
	}
	assert.True(t, add(PlanInput{Emoji: sp("🏨")}).HiddenFromPublic)
	assert.True(t, add(PlanInput{Category: sp("stay")}).HiddenFromPublic)
	assert.False(t, add(PlanInput{Category: sp("food")}).HiddenFromPublic)
	assert.False(t, add(PlanInput{Emoji: sp("🏨"), HiddenFromPublic: bp(false)}).HiddenFromPublic)

	var ve *ValidationError
	_, err := e.svc.AddPlan(e.ctx, "alice", j.Slug, d.ID, PlanInput{Description: sp("x"), Category: sp("bogus")})
	assert.ErrorAs(t, err, &ve)

	// update: category/emoji change recomputes, unrelated edit keeps stored value, explicit wins
	p := add(PlanInput{Category: sp("food")})
	got, err := e.svc.UpdatePlan(e.ctx, "alice", j.Slug, p.ID, PlanInput{Category: sp("stay")})
	require.NoError(t, err)
	assert.True(t, got.HiddenFromPublic)
	got, err = e.svc.UpdatePlan(e.ctx, "alice", j.Slug, p.ID, PlanInput{Time: sp("10:00")})
	require.NoError(t, err)
	assert.True(t, got.HiddenFromPublic)
	got, err = e.svc.UpdatePlan(e.ctx, "alice", j.Slug, p.ID, PlanInput{Category: sp("stay"), HiddenFromPublic: bp(false)})
	require.NoError(t, err)
	assert.False(t, got.HiddenFromPublic)
	got, err = e.svc.UpdatePlan(e.ctx, "alice", j.Slug, p.ID, PlanInput{Category: sp("food")})
	require.NoError(t, err)
	assert.False(t, got.HiddenFromPublic)
	got, err = e.svc.UpdatePlan(e.ctx, "alice", j.Slug, p.ID, PlanInput{Emoji: sp("🏨")})
	require.NoError(t, err)
	assert.True(t, got.HiddenFromPublic)
	_, err = e.svc.UpdatePlan(e.ctx, "alice", j.Slug, p.ID, PlanInput{Category: sp("bogus")})
	assert.ErrorAs(t, err, &ve)

	// persisted, not just returned
	reloaded := e.reload(j).ItineraryDays[0].Plans
	require.NotEmpty(t, reloaded)
}
