package services

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

// ProfileView is a user's profile page as one viewer may see it. Every trip in
// it is the viewer's JourneyView, so the privacy rules (and redaction) apply.
type ProfileView struct {
	User        models.PublicProfile `json:"user"`
	Followers   int64                `json:"followers"`
	Following   int64                `json:"following"`
	IsFollowing bool                 `json:"is_following"` // the viewer follows this user
	Now         *JourneyView         `json:"now"`          // a trip the viewer sees live; nil otherwise
	Upcoming    []JourneyView        `json:"upcoming"`
	Calendar    []CalendarItem       `json:"calendar"`
	Past        []JourneyView        `json:"past"`
	Stats       ProfileStats         `json:"stats"`
}

// CalendarItem is one trip on the travel calendar. Dates, months and companions
// are set only when the viewer's view of the trip is not redacted.
type CalendarItem struct {
	Slug          string                 `json:"slug"`
	Title         string                 `json:"title"`
	Destination   string                 `json:"destination"`
	Country       string                 `json:"country"`
	DepartureDate *time.Time             `json:"departure_date,omitempty"`
	ReturnDate    *time.Time             `json:"return_date,omitempty"`
	MonthStart    string                 `json:"month_start,omitempty"`
	MonthEnd      string                 `json:"month_end,omitempty"`
	Companions    []models.PublicProfile `json:"companions,omitempty"` // joined members other than the profile user
}

// ProfileStats counts only the completed trips the viewer can see.
type ProfileStats struct {
	Countries int `json:"countries"`
	Cities    int `json:"cities"`
	Trips     int `json:"trips"`
}

type ProfileDeps struct {
	Users    repository.UserRepository
	Journeys repository.JourneyRepository
	Members  repository.MemberRepository
	Follows  repository.FollowRepository
	Privacy  *PrivacyService
}

type ProfileService struct {
	ProfileDeps
}

func NewProfileService(d ProfileDeps) *ProfileService {
	return &ProfileService{ProfileDeps: d}
}

// profileTrip pairs a trip with the viewer's view of it.
type profileTrip struct {
	j *models.Journey
	v *JourneyView
}

// Get builds username's profile for viewerID (empty = anonymous). The trips are
// the ones the user owns or has joined and the viewer may see in lists.
//
// Nothing may reveal to a viewer that the user is away now unless the viewer
// may see it: buckets come from the (possibly masked) view status, never from
// the real dates, and a bucket holding a redacted trip is ordered by created_at.
func (s *ProfileService) Get(ctx context.Context, viewerID, username string) (*ProfileView, error) {
	u, err := s.Users.GetByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound
	}
	p := &ProfileView{
		User:     models.PublicProfile{Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL, Bio: u.Bio},
		Upcoming: []JourneyView{},
		Calendar: []CalendarItem{},
		Past:     []JourneyView{},
	}
	if p.Followers, p.Following, err = countsOf(ctx, s.Follows, u.ID); err != nil {
		return nil, err
	}
	if viewerID != "" && viewerID != u.ID {
		if p.IsFollowing, err = s.Follows.IsFollowing(ctx, viewerID, u.ID); err != nil {
			return nil, err
		}
	}

	trips, err := s.visibleTrips(ctx, viewerID, u.ID)
	if err != nil {
		return nil, err
	}
	sortByCreatedDesc(trips)

	var upcoming, past []profileTrip
	for _, t := range trips {
		switch {
		case p.Now == nil && t.v.Live != nil:
			now, err := s.nowView(ctx, viewerID, t)
			if err != nil {
				return nil, err
			}
			p.Now = now
		case t.v.Status.State == "completed":
			past = append(past, t)
		default: // planning, upcoming (incl. masked ongoing), or a second live trip
			upcoming = append(upcoming, t)
		}
	}

	orderBucket(upcoming, func(a, b profileTrip) int { return a.j.DepartureDate.Compare(b.j.DepartureDate) })
	orderBucket(past, func(a, b profileTrip) int { return b.j.ReturnDate.Compare(a.j.ReturnDate) })
	orderBucket(trips, func(a, b profileTrip) int { return a.j.DepartureDate.Compare(b.j.DepartureDate) })

	for _, t := range upcoming {
		p.Upcoming = append(p.Upcoming, *t.v)
	}
	for _, t := range past {
		p.Past = append(p.Past, *t.v)
	}
	for _, t := range trips {
		p.Calendar = append(p.Calendar, calendarItem(t.v, u.Username))
	}
	p.Stats = statsOf(p.Past)
	return p, nil
}

// visibleTrips loads the trips userID owns or has joined and keeps the ones the
// viewer may see listed. Days are not loaded.
func (s *ProfileService) visibleTrips(ctx context.Context, viewerID, userID string) ([]profileTrip, error) {
	owned, err := s.Journeys.ListByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids, err := s.Members.ListJourneyIDsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	joined, err := s.Journeys.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []profileTrip
	for _, list := range [][]models.Journey{owned, joined} {
		for i := range list {
			j := &list[i]
			if seen[j.ID] {
				continue
			}
			seen[j.ID] = true
			v, ok, err := s.Privacy.ViewOf(ctx, viewerID, j)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			rel, err := s.Privacy.Relation(ctx, viewerID, j, v.Role)
			if err != nil {
				return nil, err
			}
			if !s.Privacy.Listable(rel, j) {
				continue
			}
			out = append(out, profileTrip{j: j, v: v})
		}
	}
	return out, nil
}

// nowView returns the live trip's view; members get it reloaded with its days so
// Live carries today's plans.
func (s *ProfileService) nowView(ctx context.Context, viewerID string, t profileTrip) (*JourneyView, error) {
	if t.v.Role == "" {
		return t.v, nil
	}
	full, err := s.Privacy.View(ctx, viewerID, t.j.Slug)
	if errors.Is(err, ErrNotFound) {
		return t.v, nil // trip vanished or turned private meanwhile: keep the list view
	}
	return full, err
}

func sortByCreatedDesc(ts []profileTrip) {
	slices.SortStableFunc(ts, func(a, b profileTrip) int {
		if c := b.j.CreatedAt.Compare(a.j.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.j.ID, a.j.ID)
	})
}

// orderBucket sorts ts (already in created_at desc order) by date unless one of
// them is redacted: then the created_at order stays, so the order says nothing
// about the hidden dates.
func orderBucket(ts []profileTrip, byDate func(a, b profileTrip) int) {
	for _, t := range ts {
		if t.v.Redacted {
			return
		}
	}
	slices.SortStableFunc(ts, byDate)
}

func calendarItem(v *JourneyView, profileUser string) CalendarItem {
	it := CalendarItem{Slug: v.Slug, Title: v.Title, Destination: v.Destination, Country: v.Country}
	if v.Redacted {
		return it
	}
	it.DepartureDate, it.ReturnDate = v.DepartureDate, v.ReturnDate
	it.MonthStart, it.MonthEnd = v.MonthStart, v.MonthEnd
	for _, m := range v.Members {
		if m.Username != profileUser {
			it.Companions = append(it.Companions, models.PublicProfile{
				Username: m.Username, DisplayName: m.DisplayName, AvatarURL: m.AvatarURL,
			})
		}
	}
	return it
}

func statsOf(past []JourneyView) ProfileStats {
	countries, cities := map[string]bool{}, map[string]bool{}
	for _, v := range past {
		if c := strings.ToLower(strings.TrimSpace(v.Country)); c != "" {
			countries[c] = true
		}
		if c := strings.ToLower(strings.TrimSpace(v.Destination)); c != "" {
			cities[c] = true
		}
	}
	return ProfileStats{Countries: len(countries), Cities: len(cities), Trips: len(past)}
}
