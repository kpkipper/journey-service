package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readyMoment inserts a ready moment with its processed objects in the store.
func (e *momentEnv) readyMoment(j *models.Journey, dayID, planID, uploader string) *models.Moment {
	m := &models.Moment{JourneyID: j.ID, DayID: dayID, PlanID: planID, UploaderID: uploader,
		Kind: "photo", Status: models.MomentReady, CreatedAt: e.now}
	require.NoError(e.t, e.db.Create(m).Error)
	m.ObjectKey = storage.MomentKey(j.ID, m.ID, "full.jpg")
	m.ThumbKey = storage.MomentKey(j.ID, m.ID, "thumb.jpg")
	require.NoError(e.t, e.db.Save(m).Error)
	for _, k := range []string{m.ObjectKey, m.ThumbKey, storage.RawKey(j.ID, m.ID)} {
		require.NoError(e.t, e.store.Put(context.Background(), k, "image/jpeg", strings.NewReader("x")))
	}
	return m
}

func (e *momentEnv) setDates(j *models.Journey, dep, ret time.Time) {
	require.NoError(e.t, e.db.Model(j).Updates(map[string]any{"departure_date": dep, "return_date": ret}).Error)
}

func TestListMoments_Visibility(t *testing.T) {
	completed := [2]time.Time{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
	ongoing := [2]time.Time{time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)}
	cases := []struct {
		name, viewer, visibility string
		dates                    [2]time.Time
		follow                   bool
		wantErr                  error
		wantN                    int
	}{
		{"stranger public ongoing", "str", "public", ongoing, false, nil, 0},
		{"stranger public completed", "str", "public", completed, false, nil, 1},
		{"follower followers ongoing", "fol", "followers", ongoing, true, nil, 1},
		{"follower private", "fol", "private", ongoing, true, ErrNotFound, 0},
		{"anonymous link completed", "", "link", completed, false, nil, 1},
		{"member private", "bob", "private", ongoing, false, nil, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newMomentEnv(t)
			for _, u := range []string{"alice", "bob", "str", "fol"} {
				e.user(u)
			}
			j := e.journey("alice", true, c.visibility)
			e.setDates(j, c.dates[0], c.dates[1])
			e.member(j, "bob", models.RoleTraveler)
			if c.follow {
				require.NoError(t, e.db.Create(&models.Follow{FollowerID: "fol", FolloweeID: "alice"}).Error)
			}
			d := e.day(j)
			e.readyMoment(j, d.ID, "", "alice")
			e.seed(j, d, "alice", models.MomentPending, 2, e.now)
			e.seed(j, d, "alice", models.MomentRejected, 1, e.now)

			got, err := e.svc.List(context.Background(), c.viewer, j.Slug)
			if c.wantErr != nil {
				assert.ErrorIs(t, err, c.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Len(t, got, c.wantN)
			for _, m := range got {
				assert.Equal(t, models.MomentReady, m.Status)
			}
		})
	}
}

func TestListMoments_HiddenPlanOnlyForMembers(t *testing.T) {
	e := newMomentEnv(t)
	for _, u := range []string{"alice", "bob", "str"} {
		e.user(u)
	}
	j := e.journey("alice", true, "public")
	e.setDates(j, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	e.member(j, "bob", models.RoleTraveler)
	d := e.day(j)
	hidden := &models.ActivityPlan{ItineraryDayID: d.ID, Description: "hotel", HiddenFromPublic: true}
	require.NoError(t, e.db.Create(hidden).Error)
	pub := e.plan(d)
	mHidden := e.readyMoment(j, d.ID, hidden.ID, "alice")
	mPub := e.readyMoment(j, d.ID, pub.ID, "alice")
	mDay := e.readyMoment(j, d.ID, "", "alice")

	ids := func(v []MomentView) []string {
		var o []string
		for _, m := range v {
			o = append(o, m.ID)
		}
		return o
	}
	got, err := e.svc.List(context.Background(), "bob", j.Slug)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{mHidden.ID, mPub.ID, mDay.ID}, ids(got))
	got, err = e.svc.List(context.Background(), "str", j.Slug)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{mPub.ID, mDay.ID}, ids(got))
}

func TestListMoments_DayNotVisibleHidden(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	e.user("str")
	j := e.journey("alice", true, "public")
	e.setDates(j, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	e.day(j)
	e.readyMoment(j, "gone-day", "", "alice")
	got, err := e.svc.List(context.Background(), "str", j.Slug)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListMoments_URLsAreProcessedObjectsOnly(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	e.readyMoment(j, d.ID, "", "alice")
	got, err := e.svc.List(context.Background(), "alice", j.Slug)
	require.NoError(t, err)
	require.Len(t, got, 1)
	for _, u := range []string{got[0].URL, got[0].ThumbURL} {
		assert.Contains(t, u, "moments/")
		assert.NotContains(t, u, "uploads/")
	}
	assert.True(t, got[0].CanDelete)
}

func TestDeleteMoment_Rules(t *testing.T) {
	e := newMomentEnv(t)
	for _, u := range []string{"alice", "bob", "ed", "gone"} {
		e.user(u)
	}
	j := e.journey("alice", true, "")
	e.member(j, "bob", models.RoleTraveler)
	e.member(j, "ed", models.RoleEditor)
	d := e.day(j)
	ctx := context.Background()

	// uploader who left the trip can delete; files vanish
	m := e.readyMoment(j, d.ID, "", "gone")
	require.NoError(t, e.svc.Delete(ctx, "gone", m.ID))
	got, err := e.repo.Get(ctx, m.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Empty(t, e.store.Keys())

	// owner deletes someone else's
	m = e.readyMoment(j, d.ID, "", "bob")
	require.NoError(t, e.svc.Delete(ctx, "alice", m.ID))

	// editor cannot delete someone else's; stranger neither; missing id
	m = e.readyMoment(j, d.ID, "", "bob")
	assert.ErrorIs(t, e.svc.Delete(ctx, "ed", m.ID), ErrMomentNotFound)
	assert.ErrorIs(t, e.svc.Delete(ctx, "gone", m.ID), ErrMomentNotFound)
	assert.ErrorIs(t, e.svc.Delete(ctx, "alice", "nope"), ErrMomentNotFound)
	got, _ = e.repo.Get(ctx, m.ID)
	assert.NotNil(t, got)

	// pending: raw object removed too
	p := &models.Moment{JourneyID: j.ID, DayID: d.ID, UploaderID: "bob", Kind: "photo", Status: models.MomentPending, CreatedAt: e.now}
	require.NoError(t, e.db.Create(p).Error)
	require.NoError(t, e.store.Put(ctx, storage.RawKey(j.ID, p.ID), "image/jpeg", strings.NewReader("x")))
	require.NoError(t, e.svc.Delete(ctx, "bob", p.ID))
	assert.NotContains(t, e.store.Keys(), storage.RawKey(j.ID, p.ID))
}

func TestUpdateCaption_Rules(t *testing.T) {
	e := newMomentEnv(t)
	for _, u := range []string{"alice", "bob", "gone"} {
		e.user(u)
	}
	j := e.journey("alice", true, "")
	e.member(j, "bob", models.RoleTraveler)
	d := e.day(j)
	ctx := context.Background()
	m := e.readyMoment(j, d.ID, "", "bob")

	v, err := e.svc.UpdateCaption(ctx, "bob", m.ID, "  nice  ")
	require.NoError(t, err)
	assert.Equal(t, "nice", v.Caption)
	got, _ := e.repo.Get(ctx, m.ID)
	assert.Equal(t, "nice", got.Caption)

	_, err = e.svc.UpdateCaption(ctx, "bob", m.ID, strings.Repeat("ก", 301))
	var ve *ValidationError
	assert.ErrorAs(t, err, &ve)

	_, err = e.svc.UpdateCaption(ctx, "alice", m.ID, "x") // owner but not uploader
	assert.ErrorIs(t, err, ErrMomentNotFound)

	left := e.readyMoment(j, d.ID, "", "gone")
	_, err = e.svc.UpdateCaption(ctx, "gone", left.ID, "x")
	assert.ErrorIs(t, err, ErrMomentNotFound)
}

func TestListMine_IncludesLeftTrips(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	e.user("gone")
	j1 := e.journey("alice", true, "")
	j2 := e.journey("alice", true, "")
	e.member(j1, "gone", models.RoleTraveler)
	d1, d2 := e.day(j1), e.day(j2)
	ctx := context.Background()
	e.readyMoment(j1, d1.ID, "", "gone")
	e.readyMoment(j2, d2.ID, "", "gone") // never a member / left
	e.seed(j1, d1, "gone", models.MomentPending, 1, e.now)

	got, err := e.svc.ListMine(ctx, "gone")
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, m := range got {
		require.NotNil(t, m.Journey)
		assert.NotEmpty(t, m.Journey.Slug)
		assert.True(t, m.CanDelete)
	}

	// soft-deleted trip disappears
	require.NoError(t, repository.NewJourneyRepository(e.db).Delete(ctx, j2.Slug))
	got, err = e.svc.ListMine(ctx, "gone")
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestCleanupPending_RemovesOldOnly(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	ctx := context.Background()
	mk := func(status string, at time.Time) *models.Moment {
		m := &models.Moment{JourneyID: j.ID, DayID: d.ID, UploaderID: "alice", Kind: "photo", Status: status, CreatedAt: at}
		require.NoError(t, e.db.Create(m).Error)
		require.NoError(t, e.store.Put(ctx, storage.RawKey(j.ID, m.ID), "image/jpeg", strings.NewReader("x")))
		return m
	}
	oldP := mk(models.MomentPending, e.now.Add(-2*time.Hour))
	newP := mk(models.MomentPending, e.now.Add(-10*time.Minute))
	oldR := mk(models.MomentRejected, e.now.Add(-2*time.Hour))
	newR := mk(models.MomentRejected, e.now.Add(-10*time.Minute))
	ready := e.readyMoment(j, d.ID, "", "alice")
	require.NoError(t, e.db.Model(ready).Update("created_at", e.now.Add(-5*time.Hour)).Error)

	n, err := e.svc.CleanupPending(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	for id, want := range map[string]bool{oldP.ID: false, oldR.ID: false, newP.ID: true, newR.ID: true, ready.ID: true} {
		got, _ := e.repo.Get(ctx, id)
		assert.Equal(t, want, got != nil, id)
	}
	assert.NotContains(t, e.store.Keys(), storage.RawKey(j.ID, oldP.ID))
	assert.Contains(t, e.store.Keys(), storage.RawKey(j.ID, newP.ID))
	assert.Contains(t, e.store.Keys(), ready.ObjectKey)
}

func TestDeleteForJourney(t *testing.T) {
	e := newMomentEnv(t)
	e.user("alice")
	j := e.journey("alice", true, "")
	d := e.day(j)
	ctx := context.Background()
	e.readyMoment(j, d.ID, "", "alice")
	vid := e.readyMoment(j, d.ID, "", "alice")
	require.NoError(t, e.db.Model(vid).Update("kind", "video").Error)
	require.NoError(t, e.store.Delete(ctx, storage.MomentKey(j.ID, vid.ID, "full.jpg")))
	require.NoError(t, e.store.Put(ctx, storage.MomentKey(j.ID, vid.ID, "video.mp4"), "video/mp4", strings.NewReader("v")))
	e.seed(j, d, "alice", models.MomentPending, 1, e.now)
	e.seed(j, d, "alice", models.MomentRejected, 1, e.now)

	require.NoError(t, e.svc.DeleteForJourney(ctx, j.ID))
	rows, err := e.repo.ListByJourney(ctx, j.ID)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Empty(t, e.store.Keys())
}

func TestListMoments_DanglingPlanShownAsDayLevelForMembers(t *testing.T) {
	e := newMomentEnv(t)
	for _, u := range []string{"alice", "bob", "str"} {
		e.user(u)
	}
	j := e.journey("alice", true, "public")
	e.setDates(j, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	e.member(j, "bob", models.RoleTraveler)
	d := e.day(j)
	m := e.readyMoment(j, d.ID, "deleted-plan", "alice")

	for _, viewer := range []string{"alice", "bob"} {
		got, err := e.svc.List(context.Background(), viewer, j.Slug)
		require.NoError(t, err)
		require.Len(t, got, 1, viewer)
		assert.Equal(t, m.ID, got[0].ID)
		assert.Equal(t, "", got[0].PlanID, "dangling plan is shown day-level")
	}
	got, err := e.svc.List(context.Background(), "str", j.Slug)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListMoments_DeletedHiddenPlanStaysMembersOnly(t *testing.T) {
	e := newMomentEnv(t)
	for _, u := range []string{"alice", "bob", "fol", "str"} {
		e.user(u)
	}
	j := e.journey("alice", true, "public")
	e.setDates(j, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC))
	e.member(j, "bob", models.RoleTraveler)
	require.NoError(t, e.db.Create(&models.Follow{FollowerID: "fol", FolloweeID: "alice"}).Error)
	d := e.day(j)
	hidden := &models.ActivityPlan{ItineraryDayID: d.ID, Description: "hotel", HiddenFromPublic: true}
	require.NoError(t, e.db.Create(hidden).Error)
	pub := e.plan(d)
	mHidden := e.readyMoment(j, d.ID, hidden.ID, "alice")
	mPub := e.readyMoment(j, d.ID, pub.ID, "alice")

	itin := NewItineraryService(e.svc.Access, repository.NewItineraryRepository(e.db), repository.NewJourneyRepository(e.db), e.repo)
	require.NoError(t, itin.DeletePlan(context.Background(), "alice", j.Slug, hidden.ID))
	require.NoError(t, itin.DeletePlan(context.Background(), "alice", j.Slug, pub.ID))

	ids := func(v []MomentView) []string {
		var o []string
		for _, m := range v {
			o = append(o, m.ID)
		}
		return o
	}
	got, err := e.svc.List(context.Background(), "bob", j.Slug)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{mHidden.ID, mPub.ID}, ids(got))
	for _, m := range got {
		assert.Equal(t, "", m.PlanID)
	}
	for _, viewer := range []string{"fol", "str", ""} {
		got, err = e.svc.List(context.Background(), viewer, j.Slug)
		require.NoError(t, err, viewer)
		assert.ElementsMatch(t, []string{mPub.ID}, ids(got), viewer)
	}
}

type countingUsers struct {
	repository.UserRepository
	byID, byIDs int
}

func (c *countingUsers) GetByID(ctx context.Context, id string) (*models.User, error) {
	c.byID++
	return c.UserRepository.GetByID(ctx, id)
}

func (c *countingUsers) GetByIDs(ctx context.Context, ids []string) ([]models.User, error) {
	c.byIDs++
	return c.UserRepository.GetByIDs(ctx, ids)
}

func TestListMoments_BatchesUploadersAndSignsStableURLs(t *testing.T) {
	e := newMomentEnv(t)
	for _, u := range []string{"alice", "bob"} {
		e.user(u)
	}
	cu := &countingUsers{UserRepository: e.svc.Users}
	e.svc.Users = cu
	j := e.journey("alice", true, "private")
	e.member(j, "bob", models.RoleTraveler)
	d := e.day(j)
	for i := 0; i < 3; i++ {
		e.readyMoment(j, d.ID, "", "alice")
		e.readyMoment(j, d.ID, "", "bob")
	}

	first, err := e.svc.List(context.Background(), "alice", j.Slug)
	require.NoError(t, err)
	require.Len(t, first, 6)
	assert.Equal(t, 1, cu.byIDs)
	assert.Equal(t, 0, cu.byID)
	for _, m := range first {
		require.NotNil(t, m.Uploader)
	}

	e.now = e.now.Add(20 * time.Minute) // same hour bucket
	second, err := e.svc.List(context.Background(), "alice", j.Slug)
	require.NoError(t, err)
	for i := range first {
		assert.Equal(t, first[i].URL, second[i].URL)
		assert.Equal(t, first[i].ThumbURL, second[i].ThumbURL)
	}

	mine, err := e.svc.ListMine(context.Background(), "alice")
	require.NoError(t, err)
	require.Len(t, mine, 3)
	assert.Equal(t, 0, cu.byID)
	assert.Equal(t, 3, cu.byIDs)
}

func TestSignedExpiryBucket(t *testing.T) {
	cases := []struct{ now, want string }{
		{"12:00:00", "13:00:00"}, // 60 min left
		{"12:29:59", "13:00:00"}, // just over 30 min left
		{"12:30:00", "13:00:00"}, // exactly 30 min left
		{"12:30:01", "14:00:00"}, // under 30 min: next hour
		{"12:59:00", "14:00:00"},
	}
	for _, c := range cases {
		now, _ := time.Parse("2006-01-02 15:04:05", "2026-11-09 "+c.now)
		want, _ := time.Parse("2006-01-02 15:04:05", "2026-11-09 "+c.want)
		got := signedExpiry(now)
		assert.True(t, got.Equal(want), "%s: got %s", c.now, got)
		assert.GreaterOrEqual(t, got.Sub(now), 30*time.Minute)
		assert.LessOrEqual(t, got.Sub(now), 90*time.Minute)
	}
}
