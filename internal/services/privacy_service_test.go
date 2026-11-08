package services

import (
	"context"
	"encoding/json"
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

type privacyFixture struct {
	db       *gorm.DB
	svc      *PrivacyService
	journeys repository.JourneyRepository
	follows  repository.FollowRepository
}

// Fixed clock: 2026-12-13 12:00 in Bangkok.
var privacyNow = time.Date(2026, 12, 13, 12, 0, 0, 0, time.FixedZone("ICT", 7*3600))

func utcDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func newPrivacyFixture(t *testing.T) *privacyFixture {
	db := testutil.NewTestDB(t)
	journeys := repository.NewJourneyRepository(db)
	members := repository.NewMemberRepository(db)
	follows := repository.NewFollowRepository(db)
	svc := NewPrivacyService(PrivacyDeps{
		Access:  NewAccessService(journeys, members),
		Follows: follows,
		Members: members,
		Users:   repository.NewUserRepository(db),
		Now:     func() time.Time { return privacyNow },
	})
	for _, u := range []string{"owner", "edtr", "trav", "inv", "invfol", "fol", "str", "trfol"} {
		require.NoError(t, db.Create(&models.User{ID: u, Email: u + "@x.com", Username: u, DisplayName: "D " + u, AvatarURL: "a/" + u}).Error)
	}
	ctx := context.Background()
	for _, f := range []string{"fol", "invfol", "trfol"} {
		require.NoError(t, follows.Follow(ctx, f, "owner"))
	}
	return &privacyFixture{db: db, svc: svc, journeys: journeys, follows: follows}
}

type tripOpts struct {
	owner      string
	visibility string
	dep, ret   time.Time
	hideLive   bool
	draft      bool
}

// newTrip creates a trip with one day per date; each day has a visible plan and a hidden stay plan.
// Members: edtr (joined editor), trav + trfol (joined travelers), inv + invfol (invited).
func (f *privacyFixture) newTrip(t *testing.T, o tripOpts) *models.Journey {
	var days []models.ItineraryDay
	for d := o.dep; !d.After(o.ret); d = d.AddDate(0, 0, 1) {
		label := d.Format("2006-01-02")
		days = append(days, models.ItineraryDay{
			Date: label, DateISO: d, Title: "day " + label,
			Plans: []models.ActivityPlan{
				{Description: "sight " + label, Category: "sight", Position: 0},
				{Description: "hotel " + label, Category: "stay", Emoji: "🏨", HiddenFromPublic: true, Position: 1},
			},
		})
	}
	j := &models.Journey{
		OwnerID: o.owner, Title: "Winter trip", Destination: "Nagoya", Country: "Japan", CoverURL: "c.jpg",
		DepartureDate: o.dep, ReturnDate: o.ret, Visibility: o.visibility, Timezone: "Asia/Bangkok",
		HideLiveStatus: o.hideLive, IsDraft: o.draft, ItineraryDays: days,
	}
	require.NoError(t, f.db.Create(j).Error)
	if o.owner != "" {
		for _, m := range []struct{ user, role, status string }{
			{"edtr", models.RoleEditor, models.StatusJoined},
			{"trav", models.RoleTraveler, models.StatusJoined},
			{"trfol", models.RoleTraveler, models.StatusJoined},
			{"inv", models.RoleEditor, models.StatusInvited},
			{"invfol", models.RoleTraveler, models.StatusInvited},
		} {
			require.NoError(t, f.db.Create(&models.JourneyMember{JourneyID: j.ID, UserID: m.user, Role: m.role, Status: m.status}).Error)
		}
	}
	return j
}

var (
	upcomingDates  = [2]time.Time{utcDay(2026, 12, 20), utcDay(2026, 12, 24)}
	ongoingDates   = [2]time.Time{utcDay(2026, 12, 12), utcDay(2026, 12, 16)} // today = day 2 of 5
	completedDates = [2]time.Time{utcDay(2026, 11, 28), utcDay(2026, 12, 2)}
)

func planDescriptions(days []models.ItineraryDay) []string {
	var out []string
	for _, d := range days {
		for _, p := range d.Plans {
			out = append(out, p.Description)
		}
	}
	return out
}

func hasHidden(days []models.ItineraryDay) bool {
	for _, d := range days {
		for _, p := range d.Plans {
			if p.HiddenFromPublic {
				return true
			}
		}
	}
	return false
}

func memberUsernames(ms []models.MemberView) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Username)
	}
	return out
}

const (
	liveNone      = "none"
	liveNoToday   = "no-today"
	liveWithToday = "with-today"
)

func TestPrivacyView(t *testing.T) {
	f := newPrivacyFixture(t)
	ctx := context.Background()

	trip := func(vis string, dates [2]time.Time, hideLive bool) *models.Journey {
		return f.newTrip(t, tripOpts{owner: "owner", visibility: vis, dep: dates[0], ret: dates[1], hideLive: hideLive})
	}
	pubUpcoming := trip(models.VisibilityPublic, upcomingDates, false)
	pubOngoing := trip(models.VisibilityPublic, ongoingDates, false)
	pubCompleted := trip(models.VisibilityPublic, completedDates, false)
	folOngoing := trip(models.VisibilityFollowers, ongoingDates, false)
	folOngoingHidden := trip(models.VisibilityFollowers, ongoingDates, true)
	folUpcoming := trip(models.VisibilityFollowers, upcomingDates, false)
	folCompleted := trip(models.VisibilityFollowers, completedDates, false)
	linkUpcoming := trip(models.VisibilityLink, upcomingDates, false)
	linkOngoing := trip(models.VisibilityLink, ongoingDates, false)
	privOngoing := trip(models.VisibilityPrivate, ongoingDates, false)
	legacyOngoing := f.newTrip(t, tripOpts{visibility: models.VisibilityFollowers, dep: ongoingDates[0], ret: ongoingDates[1]})
	legacyCompleted := f.newTrip(t, tripOpts{visibility: models.VisibilityPrivate, dep: completedDates[0], ret: completedDates[1]})
	pubDraft := f.newTrip(t, tripOpts{owner: "owner", visibility: models.VisibilityPublic, dep: ongoingDates[0], ret: ongoingDates[1], draft: true})

	tests := []struct {
		name       string
		j          *models.Journey
		viewer     string
		notFound   bool
		redacted   bool
		state      string
		role       string
		live       string
		seesHidden bool
	}{
		{"public stranger upcoming", pubUpcoming, "", false, true, "upcoming", "", liveNone, false},
		{"public signed-in stranger upcoming", pubUpcoming, "str", false, true, "upcoming", "", liveNone, false},
		{"public stranger ongoing shows upcoming", pubOngoing, "", false, true, "upcoming", "", liveNone, false},
		{"public signed-in stranger ongoing", pubOngoing, "str", false, true, "upcoming", "", liveNone, false},
		{"public stranger completed sees full minus hidden", pubCompleted, "", false, false, "completed", "", liveNone, false},
		{"public follower ongoing", pubOngoing, "fol", false, false, "ongoing", "", liveNoToday, false},
		{"followers anonymous", folOngoing, "", true, false, "", "", "", false},
		{"followers signed-in stranger", folOngoing, "str", true, false, "", "", "", false},
		{"followers stranger completed still hidden", folCompleted, "str", true, false, "", "", "", false},
		{"followers follower ongoing", folOngoing, "fol", false, false, "ongoing", "", liveNoToday, false},
		{"followers follower upcoming", folUpcoming, "fol", false, false, "upcoming", "", liveNone, false},
		{"followers follower completed", folCompleted, "fol", false, false, "completed", "", liveNone, false},
		{"followers follower hide live shows upcoming", folOngoingHidden, "fol", false, false, "upcoming", "", liveNone, false},
		{"followers member hide live still live", folOngoingHidden, "trav", false, false, "ongoing", "traveler", liveWithToday, true},
		{"link stranger upcoming", linkUpcoming, "", false, true, "upcoming", "", liveNone, false},
		{"link stranger ongoing", linkOngoing, "str", false, true, "upcoming", "", liveNone, false},
		{"private follower", privOngoing, "fol", true, false, "", "", "", false},
		{"private stranger", privOngoing, "", true, false, "", "", "", false},
		{"private editor", privOngoing, "edtr", false, false, "ongoing", "editor", liveWithToday, true},
		{"private owner", privOngoing, "owner", false, false, "ongoing", "owner", liveWithToday, true},
		{"traveler ongoing gets today plans", folOngoing, "trav", false, false, "ongoing", "traveler", liveWithToday, true},
		{"member who follows owner is member", folOngoing, "trfol", false, false, "ongoing", "traveler", liveWithToday, true},
		{"invited non-follower is stranger", folOngoing, "inv", true, false, "", "", "", false},
		{"invited non-follower on private", privOngoing, "inv", true, false, "", "", "", false},
		{"invited on public is redacted stranger", pubOngoing, "inv", false, true, "upcoming", "", liveNone, false},
		{"invited follower is follower", folOngoing, "invfol", false, false, "ongoing", "", liveNoToday, false},
		{"legacy anonymous ongoing is public redacted", legacyOngoing, "", false, true, "upcoming", "", liveNone, false},
		{"legacy follower-ish user still stranger", legacyOngoing, "fol", false, true, "upcoming", "", liveNone, false},
		{"legacy completed is public full", legacyCompleted, "str", false, false, "completed", "", liveNone, false},
		{"draft stranger is planning redacted", pubDraft, "", false, true, "planning", "", liveNone, false},
		{"draft follower is planning full", pubDraft, "fol", false, false, "planning", "", liveNone, false},
		{"draft member is planning without live", pubDraft, "trav", false, false, "planning", "traveler", liveNone, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := f.svc.View(ctx, tc.viewer, tc.j.Slug)
			if tc.notFound {
				assert.ErrorIs(t, err, ErrNotFound)
				assert.Nil(t, v)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, v)

			// Always visible.
			assert.Equal(t, tc.j.Slug, v.Slug)
			assert.Equal(t, "Winter trip", v.Title)
			assert.Equal(t, "Nagoya", v.Destination)
			assert.Equal(t, "Japan", v.Country)
			assert.Equal(t, "c.jpg", v.CoverURL)
			assert.Equal(t, tc.state, v.Status.State)
			assert.Equal(t, tc.role, v.Role)
			assert.Equal(t, tc.redacted, v.Redacted)
			if tc.j.OwnerID == "" {
				assert.Equal(t, models.PublicProfile{}, v.Owner)
				assert.Equal(t, models.VisibilityPublic, v.Visibility)
			} else {
				assert.Equal(t, "owner", v.Owner.Username)
				assert.Equal(t, "D owner", v.Owner.DisplayName)
				assert.Equal(t, "a/owner", v.Owner.AvatarURL)
				assert.Equal(t, tc.j.Visibility, v.Visibility)
			}

			if tc.redacted {
				assert.Empty(t, v.MonthStart)
				assert.Empty(t, v.MonthEnd)
				assert.Nil(t, v.DepartureDate)
				assert.Nil(t, v.ReturnDate)
				assert.Nil(t, v.Days)
				assert.Nil(t, v.Members)
				assert.Nil(t, v.Live)
				assert.Zero(t, v.Status.DaysUntil)
				assert.Zero(t, v.Status.DayNumber)
				assert.Zero(t, v.Status.TotalDays)
				return
			}

			assert.Equal(t, tc.j.DepartureDate.Format("2006-01"), v.MonthStart)
			assert.Equal(t, tc.j.ReturnDate.Format("2006-01"), v.MonthEnd)
			require.NotNil(t, v.DepartureDate)
			require.NotNil(t, v.ReturnDate)
			assert.True(t, tc.j.DepartureDate.Equal(*v.DepartureDate))
			assert.True(t, tc.j.ReturnDate.Equal(*v.ReturnDate))
			require.Len(t, v.Days, 5)
			assert.Equal(t, tc.seesHidden, hasHidden(v.Days), "hidden plans")
			for _, d := range v.Days {
				assert.Contains(t, planDescriptions([]models.ItineraryDay{d}), "sight "+d.Date)
			}

			if tc.j.OwnerID == "" {
				assert.Empty(t, v.Members)
			} else {
				assert.Equal(t, []string{"owner", "edtr", "trav", "trfol"}, memberUsernames(v.Members))
				assert.Equal(t, models.RoleOwner, v.Members[0].Role)
				for _, m := range v.Members {
					assert.Equal(t, models.StatusJoined, m.Status)
				}
			}

			switch tc.live {
			case liveNone:
				assert.Nil(t, v.Live)
			case liveNoToday:
				require.NotNil(t, v.Live)
				assert.Equal(t, LiveView{City: "Nagoya", DayNumber: 2, TotalDays: 5}, *v.Live)
			case liveWithToday:
				require.NotNil(t, v.Live)
				assert.Equal(t, "Nagoya", v.Live.City)
				assert.Equal(t, 2, v.Live.DayNumber)
				assert.Equal(t, 5, v.Live.TotalDays)
				var descs []string
				for _, p := range v.Live.TodayPlans {
					descs = append(descs, p.Description)
				}
				assert.Equal(t, []string{"sight 2026-12-13", "hotel 2026-12-13"}, descs)
			}
		})
	}

	t.Run("view does not mutate the loaded journey for members", func(t *testing.T) {
		// A follower view (hidden cut) followed by a member view must still show hidden plans.
		_, err := f.svc.View(ctx, "fol", folOngoing.Slug)
		require.NoError(t, err)
		v, err := f.svc.View(ctx, "trav", folOngoing.Slug)
		require.NoError(t, err)
		assert.True(t, hasHidden(v.Days))
	})

	t.Run("missing trip", func(t *testing.T) {
		_, err := f.svc.View(ctx, "owner", "nope-123")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("redacted view JSON has no dates, days, live or ongoing", func(t *testing.T) {
		v, err := f.svc.View(ctx, "", pubOngoing.Slug)
		require.NoError(t, err)
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		for _, k := range []string{"departure_date", "return_date", "days", "members", "live"} {
			assert.NotContains(t, m, k)
		}
		for _, k := range []string{"slug", "title", "destination", "country", "cover_url", "visibility", "owner", "status", "redacted", "role"} {
			assert.Contains(t, m, k)
		}
		assert.NotContains(t, string(raw), "ongoing")
		assert.NotContains(t, string(raw), "2026-12")
		assert.NotContains(t, string(raw), "hotel")
		assert.Equal(t, true, m["redacted"])
		assert.Equal(t, map[string]any{"state": "upcoming", "days_until": 0.0, "day_number": 0.0, "total_days": 0.0}, m["status"])
	})

	t.Run("cross-month ongoing trip leaks no month range to strangers", func(t *testing.T) {
		// Nov 28 - Dec 16 viewed Dec 13: month_start 2026-11 + "upcoming" would imply "away now".
		cross := f.newTrip(t, tripOpts{owner: "owner", visibility: models.VisibilityPublic, dep: utcDay(2026, 11, 28), ret: utcDay(2026, 12, 16)})
		for _, viewer := range []string{"", "str", "inv"} {
			v, err := f.svc.View(ctx, viewer, cross.Slug)
			require.NoError(t, err)
			assert.True(t, v.Redacted)
			raw, err := json.Marshal(v)
			require.NoError(t, err)
			var m map[string]any
			require.NoError(t, json.Unmarshal(raw, &m))
			assert.NotContains(t, m, "month_start")
			assert.NotContains(t, m, "month_end")
			assert.NotContains(t, string(raw), "ongoing")
			assert.NotContains(t, string(raw), "2026-11")
			assert.NotContains(t, string(raw), "2026-12")
		}
		// A follower still sees the months.
		v, err := f.svc.View(ctx, "fol", cross.Slug)
		require.NoError(t, err)
		assert.Equal(t, "2026-11", v.MonthStart)
		assert.Equal(t, "2026-12", v.MonthEnd)
	})

	t.Run("hide live status hides day X of Y from followers but keeps dates", func(t *testing.T) {
		v, err := f.svc.View(ctx, "fol", folOngoingHidden.Slug)
		require.NoError(t, err)
		assert.False(t, v.Redacted)
		assert.Equal(t, tripstatus.Status{State: "upcoming"}, v.Status)
		assert.Nil(t, v.Live)
		require.NotNil(t, v.DepartureDate)
		require.NotNil(t, v.ReturnDate)
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "ongoing")
		assert.NotContains(t, string(raw), `"live"`)

		// Members still get the real status.
		v, err = f.svc.View(ctx, "trav", folOngoingHidden.Slug)
		require.NoError(t, err)
		assert.Equal(t, tripstatus.Status{State: "ongoing", DayNumber: 2, TotalDays: 5}, v.Status)
	})

	t.Run("full view JSON uses snake_case keys", func(t *testing.T) {
		v, err := f.svc.View(ctx, "trav", folOngoing.Slug)
		require.NoError(t, err)
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(raw, &m))
		for _, k := range []string{"departure_date", "return_date", "days", "members", "live", "month_start", "month_end", "cover_url"} {
			assert.Contains(t, m, k)
		}
		live := m["live"].(map[string]any)
		for _, k := range []string{"city", "day_number", "total_days", "today_plans"} {
			assert.Contains(t, live, k)
		}
	})

	t.Run("follower live JSON has no today_plans", func(t *testing.T) {
		v, err := f.svc.View(ctx, "fol", folOngoing.Slug)
		require.NoError(t, err)
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "today_plans")
		assert.NotContains(t, string(raw), "hotel")
	})
}

func TestPrivacyViewOf(t *testing.T) {
	f := newPrivacyFixture(t)
	ctx := context.Background()
	fol := f.newTrip(t, tripOpts{owner: "owner", visibility: models.VisibilityFollowers, dep: ongoingDates[0], ret: ongoingDates[1]})
	j, err := f.journeys.GetBySlug(ctx, fol.Slug)
	require.NoError(t, err)

	v, ok, err := f.svc.ViewOf(ctx, "str", j)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, v)

	v, ok, err = f.svc.ViewOf(ctx, "fol", j)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "ongoing", v.Status.State)
	assert.False(t, hasHidden(v.Days))

	v, ok, err = f.svc.ViewOf(ctx, "trav", j)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, models.RoleTraveler, v.Role)
	assert.True(t, hasHidden(v.Days))
	require.NotNil(t, v.Live)
	assert.Len(t, v.Live.TodayPlans, 2)

	v, ok, err = f.svc.ViewOf(ctx, "invfol", j)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Empty(t, v.Role)
	require.NotNil(t, v.Live)
	assert.Nil(t, v.Live.TodayPlans)
}

func TestPrivacyRelationAndCanSee(t *testing.T) {
	f := newPrivacyFixture(t)
	ctx := context.Background()
	owned := &models.Journey{OwnerID: "owner", Visibility: models.VisibilityFollowers}
	legacy := &models.Journey{Visibility: models.VisibilityPrivate}

	rels := []struct {
		viewer, role, want string
		j                  *models.Journey
	}{
		{"owner", models.RoleOwner, RelMember, owned},
		{"trfol", models.RoleTraveler, RelMember, owned},
		{"fol", "", RelFollower, owned},
		{"invfol", "", RelFollower, owned},
		{"str", "", RelStranger, owned},
		{"", "", RelStranger, owned},
		{"edtr", models.RoleEditor, RelStranger, legacy},
		{"fol", "", RelStranger, legacy},
	}
	for _, r := range rels {
		got, err := f.svc.Relation(ctx, r.viewer, r.j, r.role)
		require.NoError(t, err)
		assert.Equal(t, r.want, got, "viewer %q role %q legacy %v", r.viewer, r.role, r.j.OwnerID == "")
	}

	can := map[string]map[string]bool{
		models.VisibilityPublic:    {RelMember: true, RelFollower: true, RelStranger: true},
		models.VisibilityLink:      {RelMember: true, RelFollower: true, RelStranger: true},
		models.VisibilityFollowers: {RelMember: true, RelFollower: true, RelStranger: false},
		models.VisibilityPrivate:   {RelMember: true, RelFollower: false, RelStranger: false},
		"bogus":                    {RelMember: true, RelFollower: false, RelStranger: false},
	}
	for vis, byRel := range can {
		for rel, want := range byRel {
			j := &models.Journey{OwnerID: "owner", Visibility: vis}
			assert.Equal(t, want, f.svc.CanSee(rel, j), "%s/%s", vis, rel)
			assert.True(t, f.svc.CanSee(rel, &models.Journey{Visibility: vis}), "legacy %s/%s", vis, rel)
		}
	}
}

func TestPrivacyListable(t *testing.T) {
	f := newPrivacyFixture(t)
	want := map[string]map[string]bool{
		models.VisibilityPublic:    {RelMember: true, RelFollower: true, RelStranger: true},
		models.VisibilityLink:      {RelMember: true, RelFollower: false, RelStranger: false},
		models.VisibilityFollowers: {RelMember: true, RelFollower: true, RelStranger: false},
		models.VisibilityPrivate:   {RelMember: true, RelFollower: false, RelStranger: false},
	}
	for vis, byRel := range want {
		for rel, w := range byRel {
			assert.Equal(t, w, f.svc.Listable(rel, &models.Journey{OwnerID: "owner", Visibility: vis}), "%s/%s", vis, rel)
			// Legacy trips are listable as public, whatever their stored visibility.
			assert.True(t, f.svc.Listable(rel, &models.Journey{Visibility: vis}), "legacy %s/%s", vis, rel)
		}
	}
}
