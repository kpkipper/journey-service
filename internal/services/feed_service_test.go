package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type feedFixture struct {
	t       *testing.T
	db      *gorm.DB
	svc     *FeedService
	store   *storage.MemoryStore
	blocks  repository.BlockRepository
	now     time.Time
	created time.Time
}

// Users: me (viewer), a, b, c, str. Clock: 2026-12-13 12:00 UTC.
func newFeedFixture(t *testing.T) *feedFixture {
	db := testutil.NewTestDB(t)
	f := &feedFixture{t: t, db: db, store: storage.NewMemoryStore(),
		now: time.Date(2026, 12, 13, 12, 0, 0, 0, time.UTC), created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	journeys := repository.NewJourneyRepository(db)
	members := repository.NewMemberRepository(db)
	follows := repository.NewFollowRepository(db)
	users := repository.NewUserRepository(db)
	f.blocks = repository.NewBlockRepository(db)
	moments := repository.NewMomentRepository(db)
	now := func() time.Time { return f.now }
	access := NewAccessService(journeys, members).WithFollows(follows).WithBlocks(f.blocks)
	privacy := NewPrivacyService(PrivacyDeps{Access: access, Follows: follows, Members: members, Users: users, Now: now})
	msvc := NewMomentService(MomentDeps{Access: access, Privacy: privacy, Moments: moments,
		Itinerary: repository.NewItineraryRepository(db), Users: users, Blocks: f.blocks, Store: f.store, Now: now})
	for _, u := range []string{"me", "a", "b", "c", "str"} {
		require.NoError(t, db.Create(&models.User{ID: u, Email: u + "@x.com", Username: u, DisplayName: "D " + u}).Error)
	}
	f.svc = NewFeedService(FeedDeps{Journeys: journeys, Members: members, Follows: follows, Blocks: f.blocks,
		Users: users, Moments: moments, Privacy: privacy, MomentSvc: msvc, Clock: now})
	return f
}

func (f *feedFixture) follow(follower, followee string) {
	require.NoError(f.t, f.db.Create(&models.Follow{FollowerID: follower, FolloweeID: followee}).Error)
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

type fTrip struct {
	owner, vis, tz string
	dep, ret       time.Time
	draft, hide    bool
	created        time.Time
	days           int
}

func (f *feedFixture) trip(o fTrip) *models.Journey {
	if o.vis == "" {
		o.vis = "public"
	}
	if o.tz == "" {
		o.tz = "Asia/Bangkok"
	}
	if o.created.IsZero() {
		o.created = f.created
		f.created = f.created.Add(time.Hour)
	}
	j := &models.Journey{OwnerID: o.owner, Title: "T", Destination: "Nagoya", Country: "Japan", Visibility: o.vis,
		Timezone: o.tz, DepartureDate: o.dep, ReturnDate: o.ret, CreatedAt: o.created}
	require.NoError(f.t, f.db.Create(j).Error)
	require.NoError(f.t, f.db.Model(j).Updates(map[string]any{"is_draft": o.draft, "hide_live_status": o.hide}).Error)
	return j
}

func (f *feedFixture) member(j *models.Journey, user string) {
	require.NoError(f.t, f.db.Create(&models.JourneyMember{JourneyID: j.ID, UserID: user, Role: models.RoleTraveler, Status: models.StatusJoined}).Error)
}

func (f *feedFixture) dayOf(j *models.Journey, title string, hiddenPlan bool) (*models.ItineraryDay, *models.ActivityPlan) {
	d := &models.ItineraryDay{JourneyID: j.ID, Date: "d", DateISO: j.DepartureDate, Title: title}
	require.NoError(f.t, f.db.Create(d).Error)
	p := &models.ActivityPlan{ItineraryDayID: d.ID, Description: "x"}
	require.NoError(f.t, f.db.Create(p).Error)
	if hiddenPlan {
		require.NoError(f.t, f.db.Model(p).Update("hidden_from_public", true).Error)
	}
	return d, p
}

func (f *feedFixture) moment(j *models.Journey, d *models.ItineraryDay, planID, uploader string, at time.Time) {
	m := &models.Moment{JourneyID: j.ID, DayID: d.ID, PlanID: planID, UploaderID: uploader, Kind: "photo",
		Status: models.MomentReady, CreatedAt: at}
	require.NoError(f.t, f.db.Create(m).Error)
	m.ObjectKey, m.ThumbKey = storage.MomentKey(j.ID, m.ID, "full.jpg"), storage.MomentKey(j.ID, m.ID, "thumb.jpg")
	require.NoError(f.t, f.db.Save(m).Error)
	for _, k := range []string{m.ObjectKey, m.ThumbKey} {
		require.NoError(f.t, f.store.Put(context.Background(), k, "image/jpeg", strings.NewReader("x")))
	}
}

func (f *feedFixture) feed(viewer string) []FeedItem {
	p, err := f.svc.Feed(context.Background(), viewer, "", 50)
	require.NoError(f.t, err)
	return p.Items
}

func typesOf(items []FeedItem) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Type)
	}
	return out
}

func ofType(items []FeedItem, typ string) []FeedItem {
	var out []FeedItem
	for _, it := range items {
		if it.Type == typ {
			out = append(out, it)
		}
	}
	return out
}

func TestFeed_EventsFromFollowedAndMemberTrips(t *testing.T) {
	f := newFeedFixture(t)
	f.follow("me", "a")
	up := f.trip(fTrip{owner: "a", dep: day(2027, 1, 5), ret: day(2027, 1, 9)})
	f.member(up, "b")
	f.trip(fTrip{owner: "b", dep: day(2026, 12, 12), ret: day(2026, 12, 20)}) // b not followed, me not member
	f.trip(fTrip{owner: "a", dep: day(2026, 12, 10), ret: day(2026, 12, 20)}) // ongoing
	f.trip(fTrip{owner: "a", dep: day(2026, 11, 1), ret: day(2026, 11, 5)})   // completed

	items := f.feed("me")
	assert.Len(t, ofType(items, "trip_planned"), 3)
	assert.Len(t, ofType(items, "trip_started"), 2)
	assert.Len(t, ofType(items, "trip_completed"), 1)
	assert.Len(t, items, 6)
	for _, it := range items {
		assert.Equal(t, "a", it.Actor.Username)
	}
	for _, it := range ofType(items, "trip_planned") {
		if it.ID == "trip_planned:"+up.ID {
			require.Len(t, it.Companions, 1)
			assert.Equal(t, "b", it.Companions[0].Username)
		}
	}
	// newest first
	for i := 1; i < len(items); i++ {
		assert.False(t, items[i].At.After(items[i-1].At))
	}
	// no event from unfollowed b's trip
	// a member of an unfollowed owner's trip does see it
	f.member(f.trip(fTrip{owner: "c", vis: "private", dep: day(2027, 3, 1), ret: day(2027, 3, 3)}), "me")
	assert.Len(t, ofType(f.feed("me"), "trip_planned"), 4)
}

func TestFeed_ExcludesDraftLinkPrivateRedacted(t *testing.T) {
	f := newFeedFixture(t)
	f.follow("me", "a")
	d := day(2027, 1, 5)
	f.trip(fTrip{owner: "a", draft: true, dep: d, ret: d})
	f.trip(fTrip{owner: "a", vis: "link", dep: d, ret: d})
	f.trip(fTrip{owner: "a", vis: "private", dep: d, ret: d})
	assert.Empty(t, f.feed("me"))
	// stranger-view redacted: public ongoing trip of someone viewer follows is not redacted, but one the
	// viewer does NOT follow is never a source.
	f.trip(fTrip{owner: "str", dep: day(2026, 12, 12), ret: day(2026, 12, 20)})
	assert.Empty(t, f.feed("me"))
	// A member sees own link/draft? draft stays excluded, link is listable for members.
	own := f.trip(fTrip{owner: "me", vis: "link", dep: d, ret: d})
	_ = own
	f.trip(fTrip{owner: "me", draft: true, dep: d, ret: d})
	assert.Len(t, f.feed("me"), 1)
}

func TestFeed_HideLiveStatusDropsStarted(t *testing.T) {
	f := newFeedFixture(t)
	f.follow("me", "a")
	f.follow("b", "a")
	j := f.trip(fTrip{owner: "a", hide: true, dep: day(2026, 12, 10), ret: day(2026, 12, 20)})
	f.member(j, "me")
	assert.Contains(t, typesOf(f.feed("me")), "trip_started") // member
	f.db.Where("user_id = ?", "me").Delete(&models.JourneyMember{})
	assert.NotContains(t, typesOf(f.feed("me")), "trip_started") // follower
	assert.Contains(t, typesOf(f.feed("me")), "trip_planned")
}

func TestFeed_MomentsGrouped(t *testing.T) {
	f := newFeedFixture(t)
	f.follow("me", "a")
	j := f.trip(fTrip{owner: "a", dep: day(2026, 12, 10), ret: day(2026, 12, 20)})
	f.member(j, "b")
	d1, _ := f.dayOf(j, "Day one", false)
	d2, _ := f.dayOf(j, "Day two", false)
	base := time.Date(2026, 12, 11, 3, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		f.moment(j, d1, "", "b", base.Add(time.Duration(i)*time.Minute))
	}
	f.moment(j, d2, "", "b", base.Add(time.Hour))
	items := ofType(f.feed("me"), "moments_added")
	require.Len(t, items, 2)
	var g1, g2 FeedItem
	for _, it := range items {
		if it.Moments.DayID == d1.ID {
			g1 = it
		} else {
			g2 = it
		}
	}
	assert.Equal(t, 5, g1.Moments.Count)
	assert.Equal(t, "Day one", g1.Moments.DayTitle)
	assert.Len(t, g1.Moments.Thumbs, 4)
	assert.True(t, g1.At.Equal(base.Add(4*time.Minute)))
	assert.Equal(t, "b", g1.Actor.Username)
	assert.NotContains(t, g1.ID, ":b:") // id carries a hashed token, not the username
	assert.Regexp(t, `^moments_added:[^:]+:[0-9a-f]{12}:`, g1.ID)
	assert.Equal(t, 1, g2.Moments.Count)
	assert.True(t, g2.At.Equal(base.Add(time.Hour)))
}

func TestFeed_HiddenPlanMomentsNotInFollowerFeed(t *testing.T) {
	f := newFeedFixture(t)
	f.follow("me", "a")
	j := f.trip(fTrip{owner: "a", dep: day(2026, 12, 10), ret: day(2026, 12, 20)})
	d, p := f.dayOf(j, "D", true)
	f.moment(j, d, p.ID, "a", f.now.Add(-time.Hour))
	assert.Empty(t, ofType(f.feed("me"), "moments_added"))
	f.moment(j, d, "", "a", f.now.Add(-time.Hour))
	assert.Len(t, ofType(f.feed("me"), "moments_added"), 1)
	// the owner sees both
	f.follow("a", "a")
	it := ofType(f.feed("a"), "moments_added")
	require.Len(t, it, 1)
	assert.Equal(t, 2, it[0].Moments.Count)
}

func TestFeed_TimezoneBoundary(t *testing.T) {
	f := newFeedFixture(t)
	f.follow("me", "a")
	f.trip(fTrip{owner: "a", tz: "Asia/Tokyo", dep: day(2026, 12, 12), ret: day(2026, 12, 12)})
	it := ofType(f.feed("me"), "trip_started")
	require.Len(t, it, 1)
	assert.True(t, it[0].At.Equal(time.Date(2026, 12, 11, 15, 0, 0, 0, time.UTC)), it[0].At.String())
	// not yet started in Tokyo: now is 2026-12-13 12:00Z = 21:00 JST; trip ends 12-12 -> completed at 12-12 15:00Z
	assert.Len(t, ofType(f.feed("me"), "trip_completed"), 1)
	f.now = time.Date(2026, 12, 11, 14, 0, 0, 0, time.UTC)
	assert.Empty(t, ofType(f.feed("me"), "trip_started"))
}

func TestFeed_PaginationWithTies(t *testing.T) {
	f := newFeedFixture(t)
	f.follow("me", "a")
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		f.trip(fTrip{owner: "a", created: at, dep: day(2027, 1, 5), ret: day(2027, 1, 6)})
	}
	ctx := context.Background()
	var all []string
	before, pages := "", 0
	for {
		p, err := f.svc.Feed(ctx, "me", before, 2)
		require.NoError(t, err)
		pages++
		for _, it := range p.Items {
			all = append(all, it.ID)
		}
		if p.NextBefore == nil {
			break
		}
		before = *p.NextBefore
	}
	assert.Equal(t, 3, pages)
	assert.Len(t, all, 5)
	seen := map[string]bool{}
	for _, id := range all {
		assert.False(t, seen[id])
		seen[id] = true
	}
	_, err := f.svc.Feed(ctx, "me", "!!garbage", 2)
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "invalid cursor", ve.Message)
	p, err := f.svc.Feed(ctx, "me", "", 0)
	require.NoError(t, err)
	assert.Len(t, p.Items, 5)
}

func TestFeed_BlockedUsersExcluded(t *testing.T) {
	f := newFeedFixture(t)
	ctx := context.Background()
	f.follow("me", "a")
	f.follow("me", "c")
	j := f.trip(fTrip{owner: "a", dep: day(2026, 12, 10), ret: day(2026, 12, 20)})
	f.member(j, "b")
	d, _ := f.dayOf(j, "D", false)
	f.moment(j, d, "", "b", f.now.Add(-time.Hour))
	f.trip(fTrip{owner: "c", dep: day(2027, 1, 5), ret: day(2027, 1, 6)})
	assert.Len(t, ofType(f.feed("me"), "moments_added"), 1)
	// me blocks b: moments and companion gone
	require.NoError(t, f.blocks.Block(ctx, "me", "b"))
	items := f.feed("me")
	assert.Empty(t, ofType(items, "moments_added"))
	for _, it := range items {
		for _, c := range it.Companions {
			assert.NotEqual(t, "b", c.Username)
		}
	}
	// c blocks me: c's trip gone
	require.NoError(t, f.blocks.Block(ctx, "c", "me"))
	for _, it := range f.feed("me") {
		assert.NotEqual(t, "c", it.Actor.Username)
	}
	// me is a member of a's trip; a blocked: actor events dropped even for members
	require.NoError(t, f.blocks.Block(ctx, "me", "a"))
	f.member(j, "me")
	for _, it := range f.feed("me") {
		assert.NotEqual(t, "a", it.Actor.Username)
	}
}

func TestNow_ListsFollowedTravelers(t *testing.T) {
	f := newFeedFixture(t)
	ctx := context.Background()
	f.follow("me", "a")
	f.follow("me", "b")
	f.follow("me", "c")
	// a ongoing (day 4 of 11)
	f.trip(fTrip{owner: "a", dep: day(2026, 12, 10), ret: day(2026, 12, 20)})
	// a older ongoing trip: only one entry per person
	f.trip(fTrip{owner: "a", dep: day(2026, 12, 1), ret: day(2026, 12, 30)})
	// b hides live
	f.trip(fTrip{owner: "b", hide: true, dep: day(2026, 12, 10), ret: day(2026, 12, 20)})
	// c is joined member on str's trip (str not followed), which me has also joined
	cj := f.trip(fTrip{owner: "str", dep: day(2026, 12, 12), ret: day(2026, 12, 20)})
	f.member(cj, "c")
	f.member(cj, "me") // a stranger would not see the trip as live
	// str (not followed) and me
	f.trip(fTrip{owner: "str", dep: day(2026, 12, 10), ret: day(2026, 12, 20)})
	f.trip(fTrip{owner: "me", dep: day(2026, 12, 10), ret: day(2026, 12, 20)})

	got, err := f.svc.Now(ctx, "me")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "c", got[0].User.Username) // latest departure first
	assert.Equal(t, "a", got[1].User.Username)
	assert.Equal(t, 4, got[1].DayNumber)
	assert.Equal(t, 11, got[1].TotalDays)
	assert.Equal(t, day(2026, 12, 10), *got[1].Trip.DepartureDate)

	require.NoError(t, f.blocks.Block(ctx, "me", "c"))
	got, err = f.svc.Now(ctx, "me")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].User.Username)
}

func TestFeed_DatelessTripHasNoStartedOrCompleted(t *testing.T) {
	f := newFeedFixture(t)
	f.follow("me", "a")
	f.trip(fTrip{owner: "a"}) // zero departure/return dates
	items := f.feed("me")
	assert.Equal(t, []string{"trip_planned"}, typesOf(items))
}
