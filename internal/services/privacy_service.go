package services

import (
	"context"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/tripstatus"
)

// Viewer relation to a trip, strongest first.
const (
	RelMember   = "member"
	RelFollower = "follower"
	RelStranger = "stranger"
)

// JourneyView is a trip as one viewer may see it. Redacted views (stranger,
// trip not completed) omit the empty fields (months, dates, days, members, live)
// entirely from JSON.
type JourneyView struct {
	Slug          string                `json:"slug"`
	Title         string                `json:"title"`
	Destination   string                `json:"destination"`
	Country       string                `json:"country"`
	CoverURL      string                `json:"cover_url"`
	Visibility    string                `json:"visibility"`
	Owner         models.PublicProfile  `json:"owner"`
	Status        tripstatus.Status     `json:"status"`
	MonthStart    string                `json:"month_start,omitempty"` // "YYYY-MM"; empty when redacted
	MonthEnd      string                `json:"month_end,omitempty"`   // "YYYY-MM"; empty when redacted
	DepartureDate *time.Time            `json:"departure_date,omitempty"`
	ReturnDate    *time.Time            `json:"return_date,omitempty"`
	Days          []models.ItineraryDay `json:"days,omitempty"`
	Members       []models.MemberView   `json:"members,omitempty"`
	Redacted      bool                  `json:"redacted"`
	Role          string                `json:"role"`
	Live          *LiveView             `json:"live,omitempty"`
}

// LiveView is "on the trip right now" info. TodayPlans is filled for members only.
type LiveView struct {
	City       string                `json:"city"`
	DayNumber  int                   `json:"day_number"`
	TotalDays  int                   `json:"total_days"`
	TodayPlans []models.ActivityPlan `json:"today_plans,omitempty"`
}

type PrivacyDeps struct {
	Access  *AccessService
	Follows repository.FollowRepository
	Members repository.MemberRepository
	Users   repository.UserRepository
	Now     func() time.Time // defaults to time.Now
}

// PrivacyService is the single place that decides who sees what of a trip.
type PrivacyService struct {
	PrivacyDeps
}

func NewPrivacyService(d PrivacyDeps) *PrivacyService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &PrivacyService{PrivacyDeps: d}
}

// Relation classifies the viewer. role is the viewer's Access.Role. On a legacy
// trip (no owner) everyone is a stranger.
func (s *PrivacyService) Relation(ctx context.Context, viewerID string, j *models.Journey, role string) (string, error) {
	if j.OwnerID == "" {
		return RelStranger, nil
	}
	if role != "" {
		return RelMember, nil
	}
	if viewerID == "" {
		return RelStranger, nil
	}
	following, err := s.Follows.IsFollowing(ctx, viewerID, j.OwnerID)
	if err != nil {
		return "", err
	}
	if following {
		return RelFollower, nil
	}
	return RelStranger, nil
}

// CanSee reports whether a viewer with relation rel may see the trip at all.
// Legacy trips are public; unknown visibility values fail closed (members only).
func (s *PrivacyService) CanSee(rel string, j *models.Journey) bool {
	if j.OwnerID == "" {
		return true
	}
	switch j.Visibility {
	case models.VisibilityPublic, models.VisibilityLink:
		return true
	case models.VisibilityFollowers:
		return rel == RelMember || rel == RelFollower
	default:
		return rel == RelMember
	}
}

// Listable reports whether the trip may appear in lists for this viewer: link
// trips are listed only for members. Legacy trips are listable as public.
func (s *PrivacyService) Listable(rel string, j *models.Journey) bool {
	if !s.CanSee(rel, j) {
		return false
	}
	return j.OwnerID == "" || j.Visibility != models.VisibilityLink || rel == RelMember
}

// View loads the trip by slug for the viewer. A trip the viewer may not see is ErrNotFound.
func (s *PrivacyService) View(ctx context.Context, viewerID, slug string) (*JourneyView, error) {
	a, err := s.Access.Load(ctx, slug, viewerID)
	if err != nil {
		return nil, err
	}
	v, ok, err := s.view(ctx, viewerID, a)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	return v, nil
}

// ViewOf builds the view of an already-loaded trip. ok means the viewer may open
// the trip by slug — not that it may appear in lists; list endpoints must also
// check Listable. Days come from j.ItineraryDays as loaded.
func (s *PrivacyService) ViewOf(ctx context.Context, viewerID string, j *models.Journey) (*JourneyView, bool, error) {
	a, err := s.Access.ForJourney(ctx, j, viewerID)
	if err != nil {
		return nil, false, err
	}
	return s.view(ctx, viewerID, a)
}

func (s *PrivacyService) view(ctx context.Context, viewerID string, a *Access) (*JourneyView, bool, error) {
	j := a.Journey
	rel, err := s.Relation(ctx, viewerID, j, a.Role)
	if err != nil {
		return nil, false, err
	}
	if !s.CanSee(rel, j) {
		return nil, false, nil
	}

	owner, err := s.ownerProfile(ctx, j)
	if err != nil {
		return nil, false, err
	}
	status := tripstatus.Compute(j.DepartureDate, j.ReturnDate, j.Timezone, j.IsDraft, s.Now())

	v := &JourneyView{
		Slug: j.Slug, Title: j.Title, Destination: j.Destination, Country: j.Country, CoverURL: j.CoverURL,
		Visibility: j.Visibility,
		Owner:      owner.profile,
		Role:       a.Role,
	}
	if j.OwnerID == "" {
		v.Visibility = models.VisibilityPublic
	}

	if rel == RelStranger && status.State != "completed" {
		// Never reveal that the trip is happening now: no exact dates, no month
		// range (a past month_start + "upcoming" would imply "away now"), no
		// counters. Only planning (draft) or upcoming is ever shown.
		state := "upcoming"
		if j.IsDraft {
			state = "planning"
		}
		v.Status = tripstatus.Status{State: state}
		v.Redacted = true
		return v, true, nil
	}

	start, end := j.DepartureDate.UTC(), j.ReturnDate.UTC()
	if end.Before(start) {
		end = start
	}
	v.MonthStart, v.MonthEnd = start.Format("2006-01"), end.Format("2006-01")
	v.Status = status
	if rel == RelFollower && j.HideLiveStatus && status.State == "ongoing" {
		// The owner hid live status: followers keep the dates but not "day X of Y".
		v.Status = tripstatus.Status{State: "upcoming"}
	}
	dep, ret := j.DepartureDate, j.ReturnDate
	v.DepartureDate, v.ReturnDate = &dep, &ret
	v.Days = filterDays(j.ItineraryDays, rel == RelMember)

	members, err := s.joinedMembers(ctx, j, owner.member)
	if err != nil {
		return nil, false, err
	}
	v.Members = members

	if status.State == "ongoing" && (rel == RelMember || (rel == RelFollower && !j.HideLiveStatus)) {
		live := &LiveView{City: j.Destination, DayNumber: status.DayNumber, TotalDays: status.TotalDays}
		if rel == RelMember {
			live.TodayPlans = todayPlans(v.Days, j.DepartureDate, status.DayNumber)
		}
		v.Live = live
	}
	return v, true, nil
}

type ownerInfo struct {
	profile models.PublicProfile
	member  *models.MemberView
}

func (s *PrivacyService) ownerProfile(ctx context.Context, j *models.Journey) (ownerInfo, error) {
	if j.OwnerID == "" {
		return ownerInfo{}, nil
	}
	u, err := s.Users.GetByID(ctx, j.OwnerID)
	if err != nil || u == nil {
		return ownerInfo{}, err
	}
	return ownerInfo{
		profile: models.PublicProfile{Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL, Bio: u.Bio},
		member: &models.MemberView{
			Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL,
			Role: models.RoleOwner, Status: models.StatusJoined,
		},
	}, nil
}

// joinedMembers returns the owner followed by joined members; invited users are never listed.
func (s *PrivacyService) joinedMembers(ctx context.Context, j *models.Journey, owner *models.MemberView) ([]models.MemberView, error) {
	var out []models.MemberView
	if owner != nil {
		out = append(out, *owner)
	}
	rows, err := s.Members.ListByJourney(ctx, j.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Status == models.StatusJoined {
			out = append(out, r)
		}
	}
	return out, nil
}

// filterDays copies the days (never mutating the loaded journey); non-members lose hidden plans.
func filterDays(days []models.ItineraryDay, member bool) []models.ItineraryDay {
	out := make([]models.ItineraryDay, len(days))
	for i, d := range days {
		plans := make([]models.ActivityPlan, 0, len(d.Plans))
		for _, p := range d.Plans {
			if member || !p.HiddenFromPublic {
				plans = append(plans, p)
			}
		}
		d.Plans = plans
		out[i] = d
	}
	return out
}

// todayPlans returns the plans of the trip day matching status.DayNumber, i.e. the
// UTC calendar date departure+DayNumber-1 — the same calendar math tripstatus uses.
func todayPlans(days []models.ItineraryDay, departure time.Time, dayNumber int) []models.ActivityPlan {
	y, m, d := departure.UTC().Date()
	ty, tm, td := time.Date(y, m, d+dayNumber-1, 0, 0, 0, 0, time.UTC).Date()
	var out []models.ActivityPlan
	for _, day := range days {
		dy, dm, dd := day.DateISO.UTC().Date()
		if dy == ty && dm == tm && dd == td {
			out = append(out, day.Plans...)
		}
	}
	return out
}
