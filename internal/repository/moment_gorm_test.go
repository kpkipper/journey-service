package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func mk(t *testing.T, repo MomentRepository, m models.Moment, at time.Time) *models.Moment {
	t.Helper()
	m.CreatedAt = at
	require.NoError(t, repo.Create(context.Background(), &m))
	return &m
}

func TestMomentRepository_CreateGet(t *testing.T) {
	repo := NewMomentRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	m := &models.Moment{JourneyID: "j1", DayID: "d1", UploaderID: "u1", Kind: models.MomentPhoto, Status: models.MomentPending, Caption: "hi"}
	require.NoError(t, repo.Create(ctx, m))
	assert.Len(t, m.ID, 10)

	got, err := repo.Get(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "hi", got.Caption)
	assert.Equal(t, "", got.PlanID)

	got, err = repo.Get(ctx, "nope")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestMomentRepository_MarkReadyOnce(t *testing.T) {
	repo := NewMomentRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	m := mk(t, repo, models.Moment{JourneyID: "j1", DayID: "d1", UploaderID: "u1", Kind: models.MomentVideo, Status: models.MomentPending}, time.Now())

	ready := *m
	ready.Width, ready.Height, ready.DurationMs, ready.SizeBytes = 640, 480, 1234, 999
	ready.ObjectKey, ready.ThumbKey = "o/key", "t/key"
	require.NoError(t, repo.MarkReady(ctx, &ready))

	got, _ := repo.Get(ctx, m.ID)
	assert.Equal(t, models.MomentReady, got.Status)
	assert.Equal(t, 640, got.Width)
	assert.Equal(t, int64(1234), got.DurationMs)
	assert.Equal(t, int64(999), got.SizeBytes)
	assert.Equal(t, "o/key", got.ObjectKey)
	assert.Equal(t, "t/key", got.ThumbKey)

	assert.ErrorIs(t, repo.MarkReady(ctx, &ready), gorm.ErrRecordNotFound)
	assert.ErrorIs(t, repo.MarkReady(ctx, &models.Moment{ID: "missing"}), gorm.ErrRecordNotFound)
}

func TestMomentRepository_CaptionDelete(t *testing.T) {
	repo := NewMomentRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	m := mk(t, repo, models.Moment{JourneyID: "j1", DayID: "d1", UploaderID: "u1", Status: models.MomentReady}, time.Now())
	require.NoError(t, repo.UpdateCaption(ctx, m.ID, "new"))
	got, _ := repo.Get(ctx, m.ID)
	assert.Equal(t, "new", got.Caption)
	require.NoError(t, repo.Delete(ctx, m.ID))
	got, err := repo.Get(ctx, m.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestMomentRepository_ListsAndCounts(t *testing.T) {
	repo := NewMomentRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := mk(t, repo, models.Moment{JourneyID: "j1", DayID: "d1", PlanID: "p1", UploaderID: "u1", Status: models.MomentReady}, t0)
	b := mk(t, repo, models.Moment{JourneyID: "j1", DayID: "d1", PlanID: "p1", UploaderID: "u2", Status: models.MomentReady}, t0.Add(time.Hour))
	p := mk(t, repo, models.Moment{JourneyID: "j1", DayID: "d2", UploaderID: "u1", Status: models.MomentPending}, t0.Add(2*time.Hour))
	o := mk(t, repo, models.Moment{JourneyID: "j2", DayID: "d9", UploaderID: "u1", Status: models.MomentReady}, t0.Add(3*time.Hour))

	ready, err := repo.ListReadyByJourney(ctx, "j1")
	require.NoError(t, err)
	require.Len(t, ready, 2)
	assert.Equal(t, []string{a.ID, b.ID}, []string{ready[0].ID, ready[1].ID})

	all, err := repo.ListByJourney(ctx, "j1")
	require.NoError(t, err)
	assert.Len(t, all, 3)

	up, err := repo.ListByUploader(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, up, 2, "ready only")
	assert.Equal(t, []string{o.ID, a.ID}, []string{up[0].ID, up[1].ID}, "newest first")

	n, _ := repo.CountByJourney(ctx, "j1")
	assert.Equal(t, int64(3), n)
	n, _ = repo.CountByDay(ctx, "d1")
	assert.Equal(t, int64(2), n)
	n, _ = repo.CountByDay(ctx, "d2")
	assert.Equal(t, int64(1), n)
	n, _ = repo.CountPendingByUploader(ctx, "u1")
	assert.Equal(t, int64(1), n)
	n, _ = repo.CountPendingByUploader(ctx, "u2")
	assert.Equal(t, int64(0), n)
	n, _ = repo.CountStartedSince(ctx, "u1", t0.Add(time.Hour))
	assert.Equal(t, int64(2), n)
	n, _ = repo.CountStartedSince(ctx, "u1", t0)
	assert.Equal(t, int64(3), n)

	stale, err := repo.ListPendingBefore(ctx, t0.Add(3*time.Hour))
	require.NoError(t, err)
	require.Len(t, stale, 1)
	assert.Equal(t, p.ID, stale[0].ID)
	stale, _ = repo.ListPendingBefore(ctx, t0.Add(time.Hour))
	assert.Empty(t, stale)

	require.NoError(t, repo.DetachPlan(ctx, "p1"))
	got, _ := repo.Get(ctx, a.ID)
	assert.Equal(t, "", got.PlanID)
	assert.Equal(t, "d1", got.DayID)
}

func TestJourneyMembersCanPostMigration(t *testing.T) {
	db := testutil.NewTestDB(t)
	j := &models.Journey{Title: "T", Destination: "X", DepartureDate: time.Now(), ReturnDate: time.Now()}
	require.NoError(t, db.Create(j).Error)
	var got models.Journey
	require.NoError(t, db.First(&got, "id = ?", j.ID).Error)
	assert.True(t, got.MembersCanPost)
	require.NoError(t, db.Model(&models.Journey{}).Where("id = ?", j.ID).Update("members_can_post", false).Error)
	require.NoError(t, db.First(&got, "id = ?", j.ID).Error)
	assert.False(t, got.MembersCanPost)
}
