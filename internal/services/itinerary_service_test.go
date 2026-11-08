package services

import (
	"context"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type itinEnv struct {
	t        *testing.T
	db       *gorm.DB
	svc      *ItineraryService
	journeys *JourneyService
	jrepo    repository.JourneyRepository
	ctx      context.Context
}

func newItinEnv(t *testing.T) *itinEnv {
	db := testutil.NewTestDB(t)
	jr := repository.NewJourneyRepository(db)
	access := NewAccessService(jr, repository.NewMemberRepository(db))
	e := &itinEnv{
		t: t, db: db, jrepo: jr, ctx: context.Background(),
		svc:      NewItineraryService(access, repository.NewItineraryRepository(db), jr),
		journeys: NewJourneyService(jr, access),
	}
	for _, u := range []string{"alice", "edtr", "trav", "out"} {
		require.NoError(t, db.Create(&models.User{ID: u, Email: u + "@x.com", Username: u, DisplayName: u}).Error)
	}
	return e
}

// trip creates alice's trip with editor "edtr" and traveler "trav".
func (e *itinEnv) trip() *models.Journey {
	j := &models.Journey{
		OwnerID: "alice", Title: "Trip", Destination: "Nagoya", Country: "Japan", CoverURL: "https://x.test/c.jpg",
		DepartureDate: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		ReturnDate:    time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(e.t, e.db.Create(j).Error)
	for u, role := range map[string]string{"edtr": models.RoleEditor, "trav": models.RoleTraveler} {
		require.NoError(e.t, e.db.Create(&models.JourneyMember{JourneyID: j.ID, UserID: u, Role: role, Status: models.StatusJoined}).Error)
	}
	return j
}

func (e *itinEnv) day(j *models.Journey, title string) *models.ItineraryDay {
	d, err := e.svc.AddDay(e.ctx, "alice", j.Slug, DayInput{Date: sp("Dec 1"), DateISO: tp(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)), Title: sp(title)})
	require.NoError(e.t, err)
	return d
}

func (e *itinEnv) plan(j *models.Journey, dayID, desc string) *models.ActivityPlan {
	p, err := e.svc.AddPlan(e.ctx, "alice", j.Slug, dayID, PlanInput{Description: sp(desc)})
	require.NoError(e.t, err)
	return p
}

func (e *itinEnv) reload(j *models.Journey) *models.Journey {
	got, err := e.jrepo.GetBySlug(e.ctx, j.Slug)
	require.NoError(e.t, err)
	require.NotNil(e.t, got)
	return got
}

func sp(s string) *string       { return &s }
func tp(t time.Time) *time.Time { return &t }
func bp(b bool) *bool           { return &b }

func TestPatchJourney(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()

	got, err := e.svc.PatchJourney(e.ctx, "edtr", j.Slug, JourneyPatch{Title: sp("New title")})
	require.NoError(t, err)
	assert.Equal(t, "New title", got.Title)
	assert.Equal(t, "Nagoya", got.Destination)
	assert.Equal(t, "Japan", got.Country)
	assert.Equal(t, "https://x.test/c.jpg", got.CoverURL)
	assert.Equal(t, "alice", got.OwnerID)
	assert.True(t, j.DepartureDate.Equal(got.DepartureDate))
	assert.False(t, got.EditorsCanInvite)

	_, err = e.svc.PatchJourney(e.ctx, "edtr", j.Slug, JourneyPatch{EditorsCanInvite: bp(true)})
	assert.ErrorIs(t, err, ErrForbidden)
	got, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{EditorsCanInvite: bp(true)})
	require.NoError(t, err)
	assert.True(t, got.EditorsCanInvite)

	_, err = e.svc.PatchJourney(e.ctx, "trav", j.Slug, JourneyPatch{Title: sp("x")})
	assert.ErrorIs(t, err, ErrForbidden, "trav")
	// Non-members cannot see a followers trip, so it does not exist for them.
	for _, u := range []string{"out", ""} {
		_, err = e.svc.PatchJourney(e.ctx, u, j.Slug, JourneyPatch{Title: sp("x")})
		assert.ErrorIs(t, err, ErrNotFound, u)
	}
	_, err = e.svc.PatchJourney(e.ctx, "alice", "nope", JourneyPatch{Title: sp("x")})
	assert.ErrorIs(t, err, ErrNotFound)

	var ve *ValidationError
	_, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{ReturnDate: tp(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{CoverURL: sp("http://x")})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{Title: sp("  ")})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{Destination: sp("")})
	assert.ErrorAs(t, err, &ve)
	assert.Equal(t, "New title", e.reload(j).Title)
}

func TestPatchJourneyLegacyReadOnly(t *testing.T) {
	e := newItinEnv(t)
	j := &models.Journey{Title: "Old", Destination: "Kyoto", DepartureDate: time.Now(), ReturnDate: time.Now()}
	require.NoError(t, e.db.Create(j).Error)
	_, err := e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{Title: sp("x")})
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestDayCRUD(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	var ve *ValidationError

	_, err := e.svc.AddDay(e.ctx, "edtr", j.Slug, DayInput{DateISO: tp(time.Now())})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.AddDay(e.ctx, "edtr", j.Slug, DayInput{Date: sp("Dec 1")})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.AddDay(e.ctx, "trav", j.Slug, DayInput{Date: sp("Dec 1"), DateISO: tp(time.Now())})
	assert.ErrorIs(t, err, ErrForbidden)

	d := e.day(j, "Arrive")
	require.NotEmpty(t, d.ID)
	assert.Equal(t, j.ID, d.JourneyID)
	p := e.plan(j, d.ID, "Eat")

	up, err := e.svc.UpdateDay(e.ctx, "edtr", j.Slug, d.ID, DayInput{Title: sp("Renamed")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", up.Title)
	assert.Equal(t, "Dec 1", up.Date)
	assert.Equal(t, 1, len(e.reload(j).ItineraryDays[0].Plans))

	_, err = e.svc.UpdateDay(e.ctx, "edtr", j.Slug, "nope", DayInput{Title: sp("x")})
	assert.ErrorIs(t, err, ErrDayNotFound)

	require.NoError(t, e.svc.DeleteDay(e.ctx, "edtr", j.Slug, d.ID))
	assert.Empty(t, e.reload(j).ItineraryDays)
	var n int64
	require.NoError(t, e.db.Model(&models.ActivityPlan{}).Where("id = ?", p.ID).Count(&n).Error)
	assert.Zero(t, n)
	assert.ErrorIs(t, e.svc.DeleteDay(e.ctx, "edtr", j.Slug, d.ID), ErrDayNotFound)
}

func TestPlanCRUDAndPosition(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	d := e.day(j, "D1")

	p0, p1, p2 := e.plan(j, d.ID, "a"), e.plan(j, d.ID, "b"), e.plan(j, d.ID, "c")
	assert.Equal(t, []int{0, 1, 2}, []int{p0.Position, p1.Position, p2.Position})

	require.NoError(t, e.svc.DeletePlan(e.ctx, "edtr", j.Slug, p1.ID))
	p3 := e.plan(j, d.ID, "d")
	assert.Equal(t, 3, p3.Position)

	up, err := e.svc.UpdatePlan(e.ctx, "edtr", j.Slug, p0.ID, PlanInput{Description: sp("a2"), Emoji: sp("X")})
	require.NoError(t, err)
	assert.Equal(t, "a2", up.Description)
	assert.Equal(t, "X", up.Emoji)
	assert.Equal(t, 0, up.Position)

	var ve *ValidationError
	_, err = e.svc.AddPlan(e.ctx, "edtr", j.Slug, d.ID, PlanInput{Description: sp(" ")})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.AddPlan(e.ctx, "edtr", j.Slug, d.ID, PlanInput{})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.UpdatePlan(e.ctx, "edtr", j.Slug, p0.ID, PlanInput{Description: sp("")})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.AddPlan(e.ctx, "trav", j.Slug, d.ID, PlanInput{Description: sp("z")})
	assert.ErrorIs(t, err, ErrForbidden)
	_, err = e.svc.AddPlan(e.ctx, "edtr", j.Slug, "nope", PlanInput{Description: sp("z")})
	assert.ErrorIs(t, err, ErrDayNotFound)
	assert.ErrorIs(t, e.svc.DeletePlan(e.ctx, "edtr", j.Slug, p1.ID), ErrPlanNotFound)
}

func TestMovePlan(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	d1, d2 := e.day(j, "D1"), e.day(j, "D2")
	p := e.plan(j, d1.ID, "move me")
	e.plan(j, d2.ID, "x")
	e.plan(j, d2.ID, "y")

	moved, err := e.svc.UpdatePlan(e.ctx, "edtr", j.Slug, p.ID, PlanInput{DayID: &d2.ID})
	require.NoError(t, err)
	assert.Equal(t, d2.ID, moved.ItineraryDayID)
	assert.Equal(t, 2, moved.Position)
	assert.Equal(t, "move me", moved.Description)

	_, err = e.svc.UpdatePlan(e.ctx, "edtr", j.Slug, p.ID, PlanInput{DayID: sp("nope")})
	assert.ErrorIs(t, err, ErrDayNotFound)
}

func TestReorderPlans(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	d := e.day(j, "D1")
	a, b, c := e.plan(j, d.ID, "a"), e.plan(j, d.ID, "b"), e.plan(j, d.ID, "c")

	require.NoError(t, e.svc.ReorderPlans(e.ctx, "edtr", j.Slug, d.ID, []string{c.ID, a.ID, b.ID}))
	var order []string
	for _, p := range e.reload(j).ItineraryDays[0].Plans {
		order = append(order, p.ID)
	}
	assert.Equal(t, []string{c.ID, a.ID, b.ID}, order)

	var ve *ValidationError
	assert.ErrorAs(t, e.svc.ReorderPlans(e.ctx, "edtr", j.Slug, d.ID, []string{a.ID, b.ID}), &ve)
	assert.ErrorAs(t, e.svc.ReorderPlans(e.ctx, "edtr", j.Slug, d.ID, []string{a.ID, b.ID, "zzz"}), &ve)
	assert.ErrorAs(t, e.svc.ReorderPlans(e.ctx, "edtr", j.Slug, d.ID, []string{a.ID, a.ID, b.ID}), &ve)
	assert.ErrorAs(t, e.svc.ReorderPlans(e.ctx, "edtr", j.Slug, d.ID, []string{a.ID, b.ID, c.ID, "zzz"}), &ve)
	assert.ErrorIs(t, e.svc.ReorderPlans(e.ctx, "trav", j.Slug, d.ID, []string{a.ID, b.ID, c.ID}), ErrForbidden)
	assert.ErrorIs(t, e.svc.ReorderPlans(e.ctx, "edtr", j.Slug, "nope", nil), ErrDayNotFound)
	order = nil
	for _, p := range e.reload(j).ItineraryDays[0].Plans {
		order = append(order, p.ID)
	}
	assert.Equal(t, []string{c.ID, a.ID, b.ID}, order)
}

func TestCrossTripIDs(t *testing.T) {
	e := newItinEnv(t)
	a, b := e.trip(), e.trip()
	da, db := e.day(a, "A"), e.day(b, "B")
	pa, pb := e.plan(a, da.ID, "pa"), e.plan(b, db.ID, "pb")
	before := e.reload(b)

	_, err := e.svc.UpdatePlan(e.ctx, "alice", a.Slug, pb.ID, PlanInput{Description: sp("hacked")})
	assert.ErrorIs(t, err, ErrPlanNotFound)
	assert.ErrorIs(t, e.svc.DeletePlan(e.ctx, "alice", a.Slug, pb.ID), ErrPlanNotFound)
	assert.ErrorIs(t, e.svc.DeleteDay(e.ctx, "alice", a.Slug, db.ID), ErrDayNotFound)
	_, err = e.svc.UpdateDay(e.ctx, "alice", a.Slug, db.ID, DayInput{Title: sp("hacked")})
	assert.ErrorIs(t, err, ErrDayNotFound)
	_, err = e.svc.AddPlan(e.ctx, "alice", a.Slug, db.ID, PlanInput{Description: sp("hacked")})
	assert.ErrorIs(t, err, ErrDayNotFound)
	assert.ErrorIs(t, e.svc.ReorderPlans(e.ctx, "alice", a.Slug, db.ID, []string{pb.ID}), ErrDayNotFound)
	_, err = e.svc.UpdatePlan(e.ctx, "alice", a.Slug, pa.ID, PlanInput{DayID: &db.ID})
	assert.ErrorIs(t, err, ErrDayNotFound)

	assert.Equal(t, before, e.reload(b))
	assert.Equal(t, da.ID, e.reload(a).ItineraryDays[0].Plans[0].ItineraryDayID)
}

func TestCreateIgnoresClientIDAndDeletedAt(t *testing.T) {
	e := newItinEnv(t)
	in := &models.Journey{
		ID: "victim", Title: "T", Destination: "Nagoya", DepartureDate: time.Now(), ReturnDate: time.Now(),
		DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
	}
	require.NoError(t, e.journeys.Create(e.ctx, "alice", in))
	assert.NotEqual(t, "victim", in.ID)
	got, err := e.jrepo.GetBySlug(e.ctx, in.Slug)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.DeletedAt.Valid)
}

func TestWritePathsValidateURLsAndDates(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	var ve *ValidationError
	full := func(mut func(*models.Journey)) *models.Journey {
		in := &models.Journey{Title: "T", Destination: "Nagoya", DepartureDate: j.DepartureDate, ReturnDate: j.ReturnDate}
		mut(in)
		return in
	}

	assert.ErrorAs(t, e.journeys.Update(e.ctx, "alice", j.Slug, full(func(in *models.Journey) {
		in.DepartureDate, in.ReturnDate = j.ReturnDate, j.DepartureDate
	})), &ve)
	assert.ErrorAs(t, e.journeys.Create(e.ctx, "alice", full(func(in *models.Journey) { in.CoverURL = "javascript:alert(1)" })), &ve)
	bad := func(in *models.Journey) {
		in.ItineraryDays = []models.ItineraryDay{{Date: "d", DateISO: j.DepartureDate, Plans: []models.ActivityPlan{{Description: "x", MapURL: "javascript:alert(1)"}}}}
	}
	assert.ErrorAs(t, e.journeys.Create(e.ctx, "alice", full(bad)), &ve)
	assert.ErrorAs(t, e.journeys.Update(e.ctx, "alice", j.Slug, full(bad)), &ve)
	assert.Equal(t, "Trip", e.reload(j).Title)
	require.NoError(t, e.journeys.Update(e.ctx, "alice", j.Slug, full(func(in *models.Journey) { in.Title = "ok" })))
}

func TestPlanMapURL(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	d := e.day(j, "D1")
	var ve *ValidationError

	_, err := e.svc.AddPlan(e.ctx, "edtr", j.Slug, d.ID, PlanInput{Description: sp("x"), MapURL: sp("javascript:alert(1)")})
	assert.ErrorAs(t, err, &ve)
	p, err := e.svc.AddPlan(e.ctx, "edtr", j.Slug, d.ID, PlanInput{Description: sp("x"), MapURL: sp("https://maps.google.com/?q=x")})
	require.NoError(t, err)
	_, err = e.svc.UpdatePlan(e.ctx, "edtr", j.Slug, p.ID, PlanInput{MapURL: sp("javascript:alert(1)")})
	assert.ErrorAs(t, err, &ve)
	_, err = e.svc.UpdatePlan(e.ctx, "edtr", j.Slug, p.ID, PlanInput{MapURL: sp("http://maps.test/x")})
	require.NoError(t, err)
}

func TestPatchDatesOnlyCheckedWhenPatched(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	// Pre-existing bad data: return before departure already stored.
	require.NoError(t, e.db.Model(&models.Journey{}).Where("id = ?", j.ID).Update("return_date", j.DepartureDate.Add(-48*time.Hour)).Error)
	_, err := e.svc.PatchJourney(e.ctx, "alice", j.Slug, JourneyPatch{Title: sp("fine")})
	assert.NoError(t, err)
}

func TestUpdateDoesNotResurrectDeletedRows(t *testing.T) {
	e := newItinEnv(t)
	j := e.trip()
	d := e.day(j, "D1")
	p := e.plan(j, d.ID, "x")
	itin := repository.NewItineraryRepository(e.db)
	require.NoError(t, itin.DeletePlan(e.ctx, p.ID))
	assert.ErrorIs(t, itin.UpdatePlan(e.ctx, p), gorm.ErrRecordNotFound)
	require.NoError(t, itin.DeleteDay(e.ctx, d))
	assert.ErrorIs(t, itin.UpdateDay(e.ctx, d), gorm.ErrRecordNotFound)
	assert.Empty(t, e.reload(j).ItineraryDays)
}
