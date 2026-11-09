package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reportEnv struct {
	*momentEnv
	rs *ReportService
}

func newReportEnv(t *testing.T) *reportEnv {
	e := newMomentEnv(t)
	for _, u := range []string{"alice", "bob", "rep", "other"} {
		e.user(u)
	}
	blocks := repository.NewBlockRepository(e.db)
	rs := NewReportService(ReportDeps{
		Reports: repository.NewReportRepository(e.db),
		Users:   repository.NewUserRepository(e.db),
		Privacy: e.svc.Privacy,
		Moments: e.svc,
		Blocks:  blocks,
		Now:     func() time.Time { return e.now },
	})
	return &reportEnv{momentEnv: e, rs: rs}
}

func TestReport_CreateSuccess(t *testing.T) {
	e := newReportEnv(t)
	ctx := context.Background()
	j := e.journey("alice", true, "public")
	e.setDates(j, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	m := e.readyMoment(j, e.day(j).ID, "", "alice")

	r, created, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "Alice", Reason: "spam", Note: "  bad  "})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "alice", r.TargetID) // internal id (test users use name as id)
	assert.Equal(t, "bad", r.Note)
	assert.Equal(t, models.ReportOpen, r.Status)

	r, created, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "journey", Target: j.Slug, Reason: "privacy"})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, j.ID, r.TargetID)
	assert.NotEqual(t, j.Slug, r.TargetID)

	r, created, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "moment", Target: m.ID, Reason: "inappropriate"})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, m.ID, r.TargetID)
}

func TestReport_DuplicateWhileOpenThenResolve(t *testing.T) {
	e := newReportEnv(t)
	ctx := context.Background()
	in := ReportInput{TargetType: "user", Target: "alice", Reason: "spam"}
	first, created, err := e.rs.Create(ctx, "rep", in)
	require.NoError(t, err)
	require.True(t, created)

	again, created, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "alice", Reason: "harassment"})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, again.ID)

	// another reporter is independent
	_, created, err = e.rs.Create(ctx, "other", in)
	require.NoError(t, err)
	assert.True(t, created)

	require.NoError(t, e.rs.Resolve(ctx, first.ID))
	next, created, err := e.rs.Create(ctx, "rep", in)
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, first.ID, next.ID)
}

func TestReport_NotVisible(t *testing.T) {
	e := newReportEnv(t)
	ctx := context.Background()
	priv := e.journey("alice", true, "private")
	pub := e.journey("alice", true, "public")
	e.setDates(pub, time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)) // ongoing -> redacted moments
	mPriv := e.readyMoment(priv, e.day(priv).ID, "", "alice")
	mOngoing := e.readyMoment(pub, e.day(pub).ID, "", "alice")

	_, _, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "journey", Target: priv.Slug, Reason: "spam"})
	assert.ErrorIs(t, err, ErrNotFound)
	_, _, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "journey", Target: "nope", Reason: "spam"})
	assert.ErrorIs(t, err, ErrNotFound)
	_, _, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "moment", Target: mPriv.ID, Reason: "spam"})
	assert.ErrorIs(t, err, ErrMomentNotFound)
	_, _, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "moment", Target: mOngoing.ID, Reason: "spam"})
	assert.ErrorIs(t, err, ErrMomentNotFound)
	_, _, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "moment", Target: "missing", Reason: "spam"})
	assert.ErrorIs(t, err, ErrMomentNotFound)
	_, _, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "ghost", Reason: "spam"})
	assert.ErrorIs(t, err, ErrUserNotFound)

	// the owner can still report their own private trip's moment (they see it)
	_, created, err := e.rs.Create(ctx, "alice", ReportInput{TargetType: "moment", Target: mPriv.ID, Reason: "spam"})
	require.NoError(t, err)
	assert.True(t, created)
}

func TestReport_UserRules(t *testing.T) {
	e := newReportEnv(t)
	ctx := context.Background()
	var ve *ValidationError
	_, _, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "rep", Reason: "spam"})
	assert.True(t, errors.As(err, &ve))

	// alice blocked rep: rep cannot see alice
	require.NoError(t, e.db.Create(&models.Block{BlockerID: "alice", BlockedID: "rep"}).Error)
	_, _, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "alice", Reason: "spam"})
	assert.ErrorIs(t, err, ErrUserNotFound)

	// bob blocked by rep: rep may still report bob
	require.NoError(t, e.db.Create(&models.Block{BlockerID: "rep", BlockedID: "bob"}).Error)
	_, created, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "bob", Reason: "spam"})
	require.NoError(t, err)
	assert.True(t, created)
}

func TestReport_Validation(t *testing.T) {
	e := newReportEnv(t)
	ctx := context.Background()
	cases := map[string]ReportInput{
		"reason":      {TargetType: "user", Target: "alice", Reason: "boring"},
		"empty":       {TargetType: "user", Target: "alice"},
		"note":        {TargetType: "user", Target: "alice", Reason: "spam", Note: strings.Repeat("ก", 501)},
		"target type": {TargetType: "comment", Target: "x", Reason: "spam"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			var ve *ValidationError
			_, _, err := e.rs.Create(ctx, "rep", in)
			assert.True(t, errors.As(err, &ve), "%v", err)
		})
	}
	_, _, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "alice", Reason: "spam", Note: strings.Repeat("ก", 500)})
	assert.NoError(t, err)
}

func TestReport_RateLimit(t *testing.T) {
	e := newReportEnv(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("u%02d", i)
		e.user(name)
		_, created, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: name, Reason: "spam"})
		require.NoError(t, err)
		require.True(t, created)
	}
	_, _, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "alice", Reason: "spam"})
	assert.ErrorIs(t, err, ErrTooManyRequests)
	// duplicate of an open one still returns it
	_, created, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "u00", Reason: "spam"})
	require.NoError(t, err)
	assert.False(t, created)

	e.now = e.now.Add(25 * time.Hour)
	_, created, err = e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "alice", Reason: "spam"})
	require.NoError(t, err)
	assert.True(t, created)
}

func TestReport_ListResolve(t *testing.T) {
	e := newReportEnv(t)
	ctx := context.Background()
	r, _, err := e.rs.Create(ctx, "rep", ReportInput{TargetType: "user", Target: "alice", Reason: "spam"})
	require.NoError(t, err)
	l, err := e.rs.ListOpen(ctx)
	require.NoError(t, err)
	require.Len(t, l, 1)
	require.NoError(t, e.rs.Resolve(ctx, r.ID))
	require.NoError(t, e.rs.Resolve(ctx, r.ID)) // idempotent
	l, err = e.rs.ListOpen(ctx)
	require.NoError(t, err)
	assert.Empty(t, l)
	assert.ErrorIs(t, e.rs.Resolve(ctx, "nope"), ErrReportNotFound)
}

func TestMomentAdminDelete(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "private")
	m := e.readyMoment(j, e.day(j).ID, "", "alice")
	ctx := context.Background()
	require.NoError(t, e.svc.AdminDelete(ctx, m.ID))
	got, err := e.repo.Get(ctx, m.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, e.svc.AdminDelete(ctx, m.ID), ErrMomentNotFound)
}
