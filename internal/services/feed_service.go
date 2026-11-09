package services

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/tripstatus"
)

const (
	feedDefaultLimit = 20
	feedMaxLimit     = 50
	feedMaxTrips     = 300
	feedMaxThumbs    = 4
	feedMaxCompanion = 5
)

const (
	FeedTripPlanned   = "trip_planned"
	FeedTripStarted   = "trip_started"
	FeedTripCompleted = "trip_completed"
	FeedMomentsAdded  = "moments_added"
)

// FeedTrip is the trip an event is about, as the viewer sees it.
type FeedTrip struct {
	Slug          string     `json:"slug"`
	Title         string     `json:"title"`
	Destination   string     `json:"destination"`
	Country       string     `json:"country"`
	CoverURL      string     `json:"cover_url"`
	DepartureDate *time.Time `json:"departure_date,omitempty"`
	ReturnDate    *time.Time `json:"return_date,omitempty"`
}

// FeedMoments summarises one uploader's moments on one day of a trip.
type FeedMoments struct {
	Count    int      `json:"count"`
	DayID    string   `json:"day_id"`
	DayTitle string   `json:"day_title"`
	Thumbs   []string `json:"thumbs"`
}

// FeedItem is one event, derived on read from trips the viewer may see.
type FeedItem struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	At         time.Time              `json:"at"`
	Actor      models.PublicProfile   `json:"actor"`
	Trip       FeedTrip               `json:"trip"`
	Companions []models.PublicProfile `json:"companions"`
	Moments    *FeedMoments           `json:"moments,omitempty"`
}

type FeedPage struct {
	Items      []FeedItem `json:"items"`
	NextBefore *string    `json:"next_before"`
}

// NowTraveler is a followed person who is on a trip the viewer may see as live.
type NowTraveler struct {
	User      models.PublicProfile `json:"user"`
	Trip      FeedTrip             `json:"trip"`
	DayNumber int                  `json:"day_number"`
	TotalDays int                  `json:"total_days"`
}

type FeedDeps struct {
	Journeys  repository.JourneyRepository
	Members   repository.MemberRepository
	Follows   repository.FollowRepository
	Blocks    repository.BlockRepository // optional (nil = no blocks)
	Users     repository.UserRepository
	Moments   repository.MomentRepository
	Privacy   *PrivacyService
	MomentSvc *MomentService   // owns the moment visibility rules
	Clock     func() time.Time // defaults to time.Now (a field named Now would clash with the Now method)
}

type FeedService struct {
	FeedDeps
}

func NewFeedService(d FeedDeps) *FeedService {
	if d.Clock == nil {
		d.Clock = time.Now
	}
	return &FeedService{FeedDeps: d}
}

type feedTrip struct {
	j *models.Journey
	v *JourneyView
}

// related returns the ids and usernames of everyone block-related to viewerID.
func (s *FeedService) related(ctx context.Context, viewerID string) (ids, names map[string]bool, err error) {
	ids, names = map[string]bool{}, map[string]bool{}
	if s.Blocks == nil || viewerID == "" {
		return ids, names, nil
	}
	if ids, err = s.Blocks.RelatedIDs(ctx, viewerID); err != nil || len(ids) == 0 {
		return ids, names, err
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	users, err := s.Users.GetByIDs(ctx, list)
	if err != nil {
		return nil, nil, err
	}
	for _, u := range users {
		names[u.Username] = true
	}
	return ids, names, nil
}

// visibleTrips filters candidate trips down to the ones the viewer may see in
// a list: viewable, not redacted, listable, not a draft. Newest created first.
func (s *FeedService) visibleTrips(ctx context.Context, viewerID string, js []models.Journey) ([]feedTrip, error) {
	slices.SortStableFunc(js, func(a, b models.Journey) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})
	var out []feedTrip
	seen := map[string]bool{}
	for i := range js {
		j := &js[i]
		if seen[j.ID] || j.IsDraft || j.OwnerID == "" {
			continue
		}
		seen[j.ID] = true
		if len(out) >= feedMaxTrips {
			break
		}
		v, ok, err := s.Privacy.ViewOf(ctx, viewerID, j)
		if err != nil {
			return nil, err
		}
		if !ok || v.Redacted || v.Owner == nil {
			continue
		}
		rel, err := s.Privacy.Relation(ctx, viewerID, j, v.Role)
		if err != nil {
			return nil, err
		}
		if !s.Privacy.Listable(rel, j) {
			continue
		}
		out = append(out, feedTrip{j: j, v: v})
	}
	return out, nil
}

func feedTripOf(v *JourneyView) FeedTrip {
	return FeedTrip{Slug: v.Slug, Title: v.Title, Destination: v.Destination, Country: v.Country,
		CoverURL: v.CoverURL, DepartureDate: v.DepartureDate, ReturnDate: v.ReturnDate}
}

// atMidnight is 00:00 of the calendar day of d (its stored UTC date) in loc.
func atMidnight(d time.Time, days int, loc *time.Location) time.Time {
	y, m, dd := d.UTC().Date()
	return time.Date(y, m, dd+days, 0, 0, 0, 0, loc)
}

func (s *FeedService) Feed(ctx context.Context, viewerID, before string, limit int) (*FeedPage, error) {
	if limit <= 0 {
		limit = feedDefaultLimit
	}
	limit = min(limit, feedMaxLimit)
	var curAt int64
	var curID string
	if before != "" {
		var err error
		if curAt, curID, err = parseFeedCursor(before); err != nil {
			return nil, err
		}
	}
	page := &FeedPage{Items: []FeedItem{}}
	if viewerID == "" {
		return page, nil
	}

	candidates, err := s.sourceTrips(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	trips, err := s.visibleTrips(ctx, viewerID, candidates)
	if err != nil {
		return nil, err
	}
	_, blockedNames, err := s.related(ctx, viewerID)
	if err != nil {
		return nil, err
	}

	now := s.Clock()
	var items []FeedItem
	for _, t := range trips {
		items = append(items, s.tripEvents(t, now, blockedNames)...)
	}
	momentItems, err := s.momentEvents(ctx, viewerID, trips, blockedNames)
	if err != nil {
		return nil, err
	}
	items = append(items, momentItems...)

	slices.SortFunc(items, func(a, b FeedItem) int {
		if c := b.At.Compare(a.At); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})
	if before != "" {
		items = slices.DeleteFunc(items, func(it FeedItem) bool {
			at := it.At.UnixNano()
			return at > curAt || (at == curAt && it.ID >= curID)
		})
	}
	if len(items) > limit {
		last := items[limit-1]
		c := encodeFeedCursor(last.At, last.ID)
		page.NextBefore = &c
		items = items[:limit]
	}
	page.Items = items
	return page, nil
}

// sourceTrips: trips of followed owners plus trips the viewer owns or has joined.
func (s *FeedService) sourceTrips(ctx context.Context, viewerID string) ([]models.Journey, error) {
	followees, err := s.Follows.ListFolloweeIDs(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	js, err := s.Journeys.ListByOwners(ctx, append(followees, viewerID), feedMaxTrips)
	if err != nil {
		return nil, err
	}
	ids, err := s.Members.ListJourneyIDsForUser(ctx, viewerID) // joined only
	if err != nil {
		return nil, err
	}
	joined, err := s.Journeys.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return append(js, joined...), nil
}

func (s *FeedService) tripEvents(t feedTrip, now time.Time, blocked map[string]bool) []FeedItem {
	v, j := t.v, t.j
	actor := *v.Owner
	if blocked[actor.Username] {
		return nil
	}
	companions := []models.PublicProfile{}
	for _, m := range v.Members {
		if m.Username == actor.Username || blocked[m.Username] || len(companions) >= feedMaxCompanion {
			continue
		}
		companions = append(companions, models.PublicProfile{Username: m.Username, DisplayName: m.DisplayName, AvatarURL: m.AvatarURL})
	}
	mk := func(typ string, at time.Time) FeedItem {
		return FeedItem{ID: typ + ":" + j.ID, Type: typ, At: at, Actor: actor, Trip: feedTripOf(v), Companions: companions}
	}
	loc := tripstatus.Location(j.Timezone)
	out := []FeedItem{mk(FeedTripPlanned, j.CreatedAt)}
	if started := atMidnight(j.DepartureDate, 0, loc); !started.After(now) && !(j.HideLiveStatus && v.Role == "") {
		out = append(out, mk(FeedTripStarted, started))
	}
	if done := atMidnight(j.ReturnDate, 1, loc); !done.After(now) {
		out = append(out, mk(FeedTripCompleted, done))
	}
	return out
}

// momentEvents groups the moments the viewer may see by (uploader, day, local date).
func (s *FeedService) momentEvents(ctx context.Context, viewerID string, trips []feedTrip, blocked map[string]bool) ([]FeedItem, error) {
	if len(trips) == 0 {
		return nil, nil
	}
	ids := make([]string, len(trips))
	for i, t := range trips {
		ids[i] = t.j.ID
	}
	all, err := s.Moments.ListReadyByJourneys(ctx, ids)
	if err != nil {
		return nil, err
	}
	byTrip := map[string][]models.Moment{}
	for _, m := range all {
		byTrip[m.JourneyID] = append(byTrip[m.JourneyID], m)
	}
	var out []FeedItem
	for _, t := range trips {
		ms := byTrip[t.j.ID]
		if len(ms) == 0 {
			continue
		}
		// Day titles and plan visibility need the itinerary, which the list view lacks.
		full, err := s.Journeys.GetBySlug(ctx, t.j.Slug)
		if err != nil {
			return nil, err
		}
		if full == nil {
			continue
		}
		v, ok, err := s.Privacy.ViewOf(ctx, viewerID, full)
		if err != nil {
			return nil, err
		}
		if !ok || v.Redacted {
			continue
		}
		views, err := s.MomentSvc.visibleMoments(ctx, viewerID, v, ms)
		if err != nil {
			return nil, err
		}
		titles := map[string]string{}
		for _, d := range v.Days {
			titles[d.ID] = d.Title
		}
		loc := tripstatus.Location(t.j.Timezone)
		groups := map[string]*FeedItem{}
		var order []string
		for _, mv := range views {
			if mv.Uploader == nil || blocked[mv.Uploader.Username] {
				continue
			}
			date := mv.CreatedAt.In(loc).Format("2006-01-02")
			sum := sha256.Sum256([]byte(mv.UploaderID))
			id := fmt.Sprintf("%s:%s:%s:%s:%s", FeedMomentsAdded, t.j.ID, hex.EncodeToString(sum[:])[:12], mv.DayID, date)
			g := groups[id]
			if g == nil {
				g = &FeedItem{ID: id, Type: FeedMomentsAdded, Actor: *mv.Uploader, Trip: feedTripOf(v),
					Companions: []models.PublicProfile{},
					Moments:    &FeedMoments{DayID: mv.DayID, DayTitle: titles[mv.DayID], Thumbs: []string{}}}
				groups[id] = g
				order = append(order, id)
			}
			g.Moments.Count++
			if len(g.Moments.Thumbs) < feedMaxThumbs {
				g.Moments.Thumbs = append(g.Moments.Thumbs, mv.ThumbURL)
			}
			if mv.CreatedAt.After(g.At) {
				g.At = mv.CreatedAt
			}
		}
		for _, id := range order {
			out = append(out, *groups[id])
		}
	}
	return out, nil
}

func encodeFeedCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixNano(), 10) + "|" + id))
}

func parseFeedCursor(c string) (int64, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, "", &ValidationError{Message: "invalid cursor"}
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	at, perr := strconv.ParseInt(ts, 10, 64)
	if !ok || perr != nil || id == "" {
		return 0, "", &ValidationError{Message: "invalid cursor"}
	}
	return at, id, nil
}

// Now lists the followed people who are on a trip the viewer may see as live:
// one entry per person (their latest-departing live trip), newest departure first.
func (s *FeedService) Now(ctx context.Context, viewerID string) ([]NowTraveler, error) {
	out := []NowTraveler{}
	if viewerID == "" {
		return out, nil
	}
	followees, err := s.Follows.ListFolloweeIDs(ctx, viewerID)
	if err != nil || len(followees) == 0 {
		return out, err
	}
	relIDs, _, err := s.related(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	people := map[string]bool{}
	var ids []string
	for _, id := range followees {
		if id != viewerID && !relIDs[id] {
			people[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	users, err := s.Users.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	profiles := map[string]models.PublicProfile{}
	for _, u := range users {
		profiles[u.ID] = models.PublicProfile{Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL, Bio: u.Bio}
	}

	owned, err := s.Journeys.ListByOwners(ctx, ids, feedMaxTrips)
	if err != nil {
		return nil, err
	}
	rows, err := s.Members.ListJoinedByUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	membersOf := map[string][]string{}
	var jids []string
	for _, r := range rows {
		membersOf[r.JourneyID] = append(membersOf[r.JourneyID], r.UserID)
		jids = append(jids, r.JourneyID)
	}
	joined, err := s.Journeys.ListByIDs(ctx, jids)
	if err != nil {
		return nil, err
	}
	trips, err := s.visibleTrips(ctx, viewerID, append(owned, joined...))
	if err != nil {
		return nil, err
	}

	best := map[string]feedTrip{}
	later := func(a, b *models.Journey) bool { // a departs after b
		if c := a.DepartureDate.Compare(b.DepartureDate); c != 0 {
			return c > 0
		}
		return a.ID > b.ID
	}
	for _, t := range trips {
		if t.v.Live == nil {
			continue
		}
		who := append([]string{t.j.OwnerID}, membersOf[t.j.ID]...)
		for _, p := range who {
			if !people[p] {
				continue
			}
			if cur, ok := best[p]; !ok || later(t.j, cur.j) {
				best[p] = t
			}
		}
	}
	for p, t := range best {
		out = append(out, NowTraveler{User: profiles[p], Trip: feedTripOf(t.v), DayNumber: t.v.Live.DayNumber, TotalDays: t.v.Live.TotalDays})
	}
	slices.SortFunc(out, func(a, b NowTraveler) int {
		if c := b.Trip.DepartureDate.Compare(*a.Trip.DepartureDate); c != 0 {
			return c
		}
		return cmp.Compare(a.User.Username, b.User.Username)
	})
	return out, nil
}
