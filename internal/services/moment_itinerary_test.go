package services

import (
	"context"
	"strings"
	"testing"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newItinMomentEnv(t *testing.T) (*itinEnv, repository.MomentRepository, *MomentService, *storage.MemoryStore) {
	e := newItinEnv(t)
	mr := repository.NewMomentRepository(e.db)
	access := NewAccessService(e.jrepo, repository.NewMemberRepository(e.db))
	store := storage.NewMemoryStore()
	ms := NewMomentService(MomentDeps{Access: access, Moments: mr, Users: repository.NewUserRepository(e.db), Store: store})
	e.journeys = NewJourneyService(e.jrepo, access).WithMoments(mr, ms.DeleteForJourney)
	return e, mr, ms, store
}

func addMoment(t *testing.T, e *itinEnv, j *models.Journey, dayID, planID string) *models.Moment {
	m := &models.Moment{JourneyID: j.ID, DayID: dayID, PlanID: planID, UploaderID: "alice", Kind: "photo", Status: models.MomentReady}
	require.NoError(t, e.db.Create(m).Error)
	return m
}

func TestDeleteDay_WithMomentsConflict(t *testing.T) {
	e, _, _, _ := newItinMomentEnv(t)
	j := e.trip()
	d := e.day(j, "D1")
	addMoment(t, e, j, d.ID, "")
	assert.ErrorIs(t, e.svc.DeleteDay(e.ctx, "alice", j.Slug, d.ID), ErrDayHasMoments)
	got, err := repository.NewItineraryRepository(e.db).GetDay(e.ctx, j.ID, d.ID)
	require.NoError(t, err)
	assert.NotNil(t, got)
}

func TestDeletePlan_DetachesMoments(t *testing.T) {
	e, mr, _, _ := newItinMomentEnv(t)
	j := e.trip()
	d := e.day(j, "D1")
	p := e.plan(j, d.ID, "a")
	m := addMoment(t, e, j, d.ID, p.ID)
	require.NoError(t, e.svc.DeletePlan(e.ctx, "alice", j.Slug, p.ID))
	got, err := mr.Get(e.ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "", got.PlanID)
	assert.Equal(t, d.ID, got.DayID)
}

func TestFullPut_WithMomentsConflict(t *testing.T) {
	e, _, _, _ := newItinMomentEnv(t)
	j := e.trip()
	d := e.day(j, "D1")
	addMoment(t, e, j, d.ID, "")
	err := e.journeys.Update(e.ctx, "alice", j.Slug, &models.Journey{Title: "New", Destination: "X",
		DepartureDate: j.DepartureDate, ReturnDate: j.ReturnDate})
	assert.ErrorIs(t, err, ErrTripHasMoments)
}

func TestDeleteJourney_DeletesMoments(t *testing.T) {
	e, mr, _, store := newItinMomentEnv(t)
	j := e.trip()
	d := e.day(j, "D1")
	m := addMoment(t, e, j, d.ID, "")
	key := storage.MomentKey(j.ID, m.ID, "full.jpg")
	require.NoError(t, store.Put(context.Background(), key, "image/jpeg", strings.NewReader("x")))
	require.NoError(t, e.journeys.Delete(e.ctx, "alice", j.Slug))
	got, err := mr.Get(e.ctx, m.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Empty(t, store.Keys())
}
