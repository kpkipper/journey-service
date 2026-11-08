package services

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/kpkipper/journey-service/internal/tripstatus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type profileFixture struct {
	db      *gorm.DB
	svc     *ProfileService
	follow  *FollowService
	follows repository.FollowRepository
	created time.Time // created_at of the next trip; each trip is one hour newer
}

// Users: alice (profile owner), bob, fol (follows alice), str (stranger), kmem.
// The clock is privacyNow (2026-12-13 Bangkok).
func newProfileFixture(t *testing.T) *profileFixture {
	db := testutil.NewTestDB(t)
	journeys := repository.NewJourneyRepository(db)
	members := repository.NewMemberRepository(db)
	follows := repository.NewFollowRepository(db)
	users := repository.NewUserRepository(db)
	privacy := NewPrivacyService(PrivacyDeps{
		Access:  NewAccessService(journeys, members),
		Follows: follows,
		Members: members,
		Users:   users,
		Now:     func() time.Time { return privacyNow },
	})
	for _, u := range []string{"alice", "bob", "fol", "str", "kmem"} {
		require.NoError(t, db.Create(&models.User{ID: u, Email: u + "@x.com", Username: u, DisplayName: "D " + u}).Error)
	}
	require.NoError(t, follows.Follow(context.Background(), "fol", "alice"))
	return &profileFixture{
		db: db,
		svc: NewProfileService(ProfileDeps{
			Users: users, Journeys: journeys, Members: members, Follows: follows, Privacy: privacy,
		}),
		follow:  NewFollowService(users, follows),
		follows: follows,
		created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

type pTrip struct {
	owner, visibility, dest, country string
	dates                            [2]time.Time
	hideLive                         bool
	withDays                         bool // one day per date with one plan each
}

func (f *profileFixture) trip(t *testing.T, o pTrip) *models.Journey {
	if o.dest == "" {
		o.dest = "Nagoya"
	}
	if o.country == "" {
		o.country = "Japan"
	}
	var days []models.ItineraryDay
	if o.withDays {
		for d := o.dates[0]; !d.After(o.dates[1]); d = d.AddDate(0, 0, 1) {
			days = append(days, models.ItineraryDay{
				Date: d.Format("2006-01-02"), DateISO: d,
				Plans: []models.ActivityPlan{{Description: "plan " + d.Format("2006-01-02")}},
			})
		}
	}
	j := &models.Journey{
		OwnerID: o.owner, Title: "Trip " + o.dest, Destination: o.dest, Country: o.country,
		DepartureDate: o.dates[0], ReturnDate: o.dates[1], Visibility: o.visibility, Timezone: "Asia/Bangkok",
		HideLiveStatus: o.hideLive, ItineraryDays: days, CreatedAt: f.created,
	}
	f.created = f.created.Add(time.Hour)
	require.NoError(t, f.db.Create(j).Error)
	return j
}

func (f *profileFixture) member(t *testing.T, j *models.Journey, user, status string) {
	require.NoError(t, f.db.Create(&models.JourneyMember{JourneyID: j.ID, UserID: user, Role: models.RoleTraveler, Status: status}).Error)
}

func slugsOf(vs []JourneyView) []string {
	out := []string{}
	for _, v := range vs {
		out = append(out, v.Slug)
	}
	return out
}

func calendarSlugs(items []CalendarItem) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Slug)
	}
	return out
}

func findView(vs []JourneyView, slug string) *JourneyView {
	for i := range vs {
		if vs[i].Slug == slug {
			return &vs[i]
		}
	}
	return nil
}

func findCalendar(items []CalendarItem, slug string) *CalendarItem {
	for i := range items {
		if items[i].Slug == slug {
			return &items[i]
		}
	}
	return nil
}

func TestProfile_NowHiddenFromStranger(t *testing.T) {
	f := newProfileFixture(t)
	ctx := context.Background()
	live := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dates: ongoingDates, withDays: true})
	f.member(t, live, "kmem", models.StatusJoined)

	for _, viewer := range []string{"", "str"} {
		p, err := f.svc.Get(ctx, viewer, "alice")
		require.NoError(t, err)
		assert.Nil(t, p.Now, "viewer %q", viewer)
		assert.Empty(t, p.Past)

		// The trip is only an upcoming, redacted item: no dates, months or counters.
		require.Equal(t, []string{live.Slug}, slugsOf(p.Upcoming))
		v := p.Upcoming[0]
		assert.True(t, v.Redacted)
		assert.Equal(t, tripstatus.Status{State: "upcoming"}, v.Status)
		assert.Nil(t, v.DepartureDate)
		assert.Empty(t, v.MonthStart)
		assert.Nil(t, v.Live)

		require.Len(t, p.Calendar, 1)
		c := p.Calendar[0]
		assert.Equal(t, live.Slug, c.Slug)
		assert.Nil(t, c.DepartureDate)
		assert.Nil(t, c.ReturnDate)
		assert.Empty(t, c.MonthStart)
		assert.Empty(t, c.MonthEnd)
		assert.Nil(t, c.Companions)
		assert.Equal(t, ProfileStats{}, p.Stats)

		raw, err := json.Marshal(p)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "ongoing")
		assert.NotContains(t, string(raw), "2026-12")
		assert.NotContains(t, string(raw), "kmem")
		assert.Contains(t, string(raw), `"now":null`)
	}

	// A follower sees "now" with city and day X of Y, without today's plans.
	p, err := f.svc.Get(ctx, "fol", "alice")
	require.NoError(t, err)
	require.NotNil(t, p.Now)
	assert.Equal(t, live.Slug, p.Now.Slug)
	require.NotNil(t, p.Now.Live)
	assert.Equal(t, "Nagoya", p.Now.Live.City)
	assert.Equal(t, 2, p.Now.Live.DayNumber)
	assert.Equal(t, 5, p.Now.Live.TotalDays)
	assert.Empty(t, p.Now.Live.TodayPlans)
	assert.Empty(t, p.Upcoming, "the now trip is not repeated in upcoming")
	c := findCalendar(p.Calendar, live.Slug)
	require.NotNil(t, c)
	require.NotNil(t, c.DepartureDate)
	assert.Equal(t, ongoingDates[0], *c.DepartureDate)
	assert.Equal(t, "2026-12", c.MonthStart)
	assert.Equal(t, []models.PublicProfile{{Username: "kmem", DisplayName: "D kmem"}}, c.Companions,
		"companions are the joined members minus the profile user")

	// Members (the owner and a co-traveler) also get today's plans.
	for _, viewer := range []string{"alice", "kmem"} {
		p, err := f.svc.Get(ctx, viewer, "alice")
		require.NoError(t, err)
		require.NotNil(t, p.Now, viewer)
		require.NotNil(t, p.Now.Live)
		require.Len(t, p.Now.Live.TodayPlans, 1)
		assert.Equal(t, "plan 2026-12-13", p.Now.Live.TodayPlans[0].Description)
	}
}

func TestProfile_HiddenLiveStatusFollower(t *testing.T) {
	f := newProfileFixture(t)
	live := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dates: ongoingDates, hideLive: true})

	p, err := f.svc.Get(context.Background(), "fol", "alice")
	require.NoError(t, err)
	assert.Nil(t, p.Now)
	require.Equal(t, []string{live.Slug}, slugsOf(p.Upcoming))
	assert.Equal(t, tripstatus.Status{State: "upcoming"}, p.Upcoming[0].Status)
	assert.Nil(t, p.Upcoming[0].Live)

	// The owner still sees it as now.
	p, err = f.svc.Get(context.Background(), "alice", "alice")
	require.NoError(t, err)
	require.NotNil(t, p.Now)
	assert.Equal(t, live.Slug, p.Now.Slug)
}

func TestProfile_Ordering(t *testing.T) {
	f := newProfileFixture(t)
	ctx := context.Background()
	// Created so that newest-created-first differs from date order in every bucket.
	live := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Live", dates: ongoingDates})
	soon := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Soon", dates: upcomingDates})
	late := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Late", dates: [2]time.Time{utcDay(2027, 3, 1), utcDay(2027, 3, 5)}})
	recent := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Recent", dates: completedDates})
	older := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Older", dates: [2]time.Time{utcDay(2026, 10, 1), utcDay(2026, 10, 3)}})

	// Strangers: every unfinished trip in one upcoming bucket, newest created first.
	p, err := f.svc.Get(ctx, "str", "alice")
	require.NoError(t, err)
	assert.Equal(t, []string{late.Slug, soon.Slug, live.Slug}, slugsOf(p.Upcoming))
	assert.Equal(t, []string{recent.Slug, older.Slug}, slugsOf(p.Past), "past (never redacted): latest return first")
	assert.Equal(t, []string{older.Slug, recent.Slug, late.Slug, soon.Slug, live.Slug}, calendarSlugs(p.Calendar),
		"a calendar with redacted items is in created_at order")

	// The owner: date order.
	p, err = f.svc.Get(ctx, "alice", "alice")
	require.NoError(t, err)
	assert.Equal(t, live.Slug, p.Now.Slug)
	assert.Equal(t, []string{soon.Slug, late.Slug}, slugsOf(p.Upcoming), "upcoming: nearest departure first")
	assert.Equal(t, []string{recent.Slug, older.Slug}, slugsOf(p.Past))
	assert.Equal(t, []string{older.Slug, recent.Slug, live.Slug, soon.Slug, late.Slug}, calendarSlugs(p.Calendar),
		"calendar: by departure")
}

func TestProfile_MixedBucketUsesCreatedOrder(t *testing.T) {
	f := newProfileFixture(t)
	ctx := context.Background()
	// fol follows alice but not bob; alice joined bob's public trip, which is redacted for fol.
	// bobs departs first, so date order would put it first.
	bobs := f.trip(t, pTrip{owner: "bob", visibility: models.VisibilityPublic, dest: "Bobs", dates: upcomingDates})
	f.member(t, bobs, "alice", models.StatusJoined)
	own := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Own", dates: [2]time.Time{utcDay(2027, 3, 1), utcDay(2027, 3, 5)}})

	p, err := f.svc.Get(ctx, "fol", "alice")
	require.NoError(t, err)
	require.Equal(t, []string{own.Slug, bobs.Slug}, slugsOf(p.Upcoming))
	assert.True(t, findView(p.Upcoming, bobs.Slug).Redacted)
	assert.False(t, findView(p.Upcoming, own.Slug).Redacted)
	assert.Equal(t, []string{own.Slug, bobs.Slug}, calendarSlugs(p.Calendar))
	assert.Nil(t, findCalendar(p.Calendar, bobs.Slug).DepartureDate)
	assert.NotNil(t, findCalendar(p.Calendar, own.Slug).DepartureDate)
}

func TestProfile_Stats(t *testing.T) {
	f := newProfileFixture(t)
	ctx := context.Background()
	f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Tokyo", country: "Japan", dates: completedDates})
	f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Osaka", country: " japan ", dates: completedDates})
	f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Bangkok", country: "Thailand", dates: completedDates})
	korea := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPrivate, dest: "Seoul", country: "Korea", dates: completedDates})
	f.member(t, korea, "kmem", models.StatusJoined)
	// Unfinished trips never count.
	f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Hanoi", country: "Vietnam", dates: upcomingDates})
	f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Sapporo", country: "Japan", dates: ongoingDates})

	p, err := f.svc.Get(ctx, "str", "alice")
	require.NoError(t, err)
	assert.Equal(t, ProfileStats{Countries: 2, Cities: 3, Trips: 3}, p.Stats)
	assert.Nil(t, findCalendar(p.Calendar, korea.Slug), "private trip is not on the calendar")

	p, err = f.svc.Get(ctx, "kmem", "alice")
	require.NoError(t, err)
	assert.Equal(t, ProfileStats{Countries: 3, Cities: 4, Trips: 4}, p.Stats)
}

func TestProfile_IncludesJoinedTrips(t *testing.T) {
	f := newProfileFixture(t)
	ctx := context.Background()
	joined := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dates: completedDates})
	f.member(t, joined, "bob", models.StatusJoined)
	invited := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dest: "Kyoto", dates: completedDates})
	f.member(t, invited, "bob", models.StatusInvited)

	for _, viewer := range []string{"", "str", "bob"} {
		p, err := f.svc.Get(ctx, viewer, "bob")
		require.NoError(t, err)
		assert.Equal(t, []string{joined.Slug}, slugsOf(p.Past), "viewer %q", viewer)
		assert.Equal(t, []string{joined.Slug}, calendarSlugs(p.Calendar))
		assert.Equal(t, []models.PublicProfile{{Username: "alice", DisplayName: "D alice"}}, p.Calendar[0].Companions)
	}
}

func TestProfile_ListableOnly(t *testing.T) {
	f := newProfileFixture(t)
	ctx := context.Background()
	pub := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dates: completedDates})
	fol := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityFollowers, dates: completedDates})
	link := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityLink, dates: completedDates})
	priv := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPrivate, dates: completedDates})
	deleted := f.trip(t, pTrip{owner: "alice", visibility: models.VisibilityPublic, dates: completedDates})
	require.NoError(t, f.db.Delete(deleted).Error)

	for viewer, want := range map[string][]string{
		"":      {pub.Slug},
		"str":   {pub.Slug},
		"fol":   {pub.Slug, fol.Slug},
		"alice": {pub.Slug, fol.Slug, link.Slug, priv.Slug},
	} {
		p, err := f.svc.Get(ctx, viewer, "alice")
		require.NoError(t, err)
		assert.ElementsMatch(t, want, slugsOf(p.Past), "viewer %q", viewer)
		assert.ElementsMatch(t, want, calendarSlugs(p.Calendar), "viewer %q", viewer)
		assert.Equal(t, len(want), p.Stats.Trips)
	}
}

func TestProfile_UserAndFollowCounts(t *testing.T) {
	f := newProfileFixture(t)
	ctx := context.Background()
	p, err := f.svc.Get(ctx, "fol", "ALICE")
	require.NoError(t, err)
	assert.Equal(t, models.PublicProfile{Username: "alice", DisplayName: "D alice"}, p.User)
	assert.Equal(t, int64(1), p.Followers)
	assert.Equal(t, int64(0), p.Following)
	assert.True(t, p.IsFollowing)
	assert.NotNil(t, p.Upcoming)
	assert.NotNil(t, p.Calendar)
	assert.NotNil(t, p.Past)

	for _, viewer := range []string{"", "str", "alice"} {
		p, err := f.svc.Get(ctx, viewer, "alice")
		require.NoError(t, err)
		assert.False(t, p.IsFollowing, "viewer %q", viewer)
	}

	_, err = f.svc.Get(ctx, "fol", "nobody")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestFollowService(t *testing.T) {
	f := newProfileFixture(t)
	ctx := context.Background()

	err := f.follow.Follow(ctx, "bob", "bob")
	var ve *ValidationError
	require.True(t, errors.As(err, &ve), "got %v", err)
	assert.Equal(t, "cannot follow yourself", ve.Message)
	assert.ErrorIs(t, f.follow.Follow(ctx, "bob", "nobody"), ErrUserNotFound)
	assert.ErrorIs(t, f.follow.Unfollow(ctx, "bob", "nobody"), ErrUserNotFound)

	require.NoError(t, f.follow.Follow(ctx, "bob", "alice"))
	require.NoError(t, f.follow.Follow(ctx, "bob", "Alice"), "following twice is fine")
	followers, following, err := f.follow.Counts(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, int64(2), followers) // fol + bob
	assert.Equal(t, int64(0), following)
	_, following, err = f.follow.Counts(ctx, "bob")
	require.NoError(t, err)
	assert.Equal(t, int64(1), following)

	require.NoError(t, f.follow.Unfollow(ctx, "bob", "alice"))
	require.NoError(t, f.follow.Unfollow(ctx, "bob", "alice"), "unfollowing twice is fine")
	followers, _, err = f.follow.Counts(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, int64(1), followers)

	_, _, err = f.follow.Counts(ctx, "nobody")
	assert.ErrorIs(t, err, ErrUserNotFound)
}
