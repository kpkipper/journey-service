package services

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

var ErrDayNotFound = errors.New("day not found")
var ErrPlanNotFound = errors.New("plan not found")

type JourneyPatch struct {
	Title            *string    `json:"title"`
	Destination      *string    `json:"destination"`
	Country          *string    `json:"country"`
	CoverURL         *string    `json:"cover_url"`
	DepartureDate    *time.Time `json:"departure_date"`
	ReturnDate       *time.Time `json:"return_date"`
	EditorsCanInvite *bool      `json:"editors_can_invite"`
}

type DayInput struct {
	Date    *string    `json:"date"`
	DateISO *time.Time `json:"date_iso"`
	Title   *string    `json:"title"`
}

type PlanInput struct {
	Time        *string `json:"time"`
	Description *string `json:"description"`
	Emoji       *string `json:"emoji"`
	MapURL      *string `json:"map_url"`
	DayID       *string `json:"day_id"`
}

type ItineraryService struct {
	access   *AccessService
	repo     repository.ItineraryRepository
	journeys repository.JourneyRepository
}

func NewItineraryService(a *AccessService, r repository.ItineraryRepository, j repository.JourneyRepository) *ItineraryService {
	return &ItineraryService{access: a, repo: r, journeys: j}
}

func (s *ItineraryService) editable(ctx context.Context, userID, slug string) (*Access, error) {
	a, err := s.access.Load(ctx, slug, userID)
	if err != nil {
		return nil, err
	}
	if !a.CanEdit() {
		return nil, ErrForbidden
	}
	return a, nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

func (s *ItineraryService) PatchJourney(ctx context.Context, userID, slug string, p JourneyPatch) (*models.Journey, error) {
	a, err := s.editable(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	j := a.Journey
	if p.EditorsCanInvite != nil && !a.IsOwner() {
		return nil, ErrForbidden
	}

	fields := map[string]any{}
	if p.Title != nil {
		if blank(*p.Title) {
			return nil, &ValidationError{Message: "title is required"}
		}
		fields["title"] = *p.Title
	}
	if p.Destination != nil {
		if blank(*p.Destination) {
			return nil, &ValidationError{Message: "destination is required"}
		}
		fields["destination"] = *p.Destination
	}
	if p.Country != nil {
		fields["country"] = *p.Country
	}
	if p.CoverURL != nil {
		if *p.CoverURL != "" && !validHTTPSURL(*p.CoverURL) {
			return nil, &ValidationError{Message: "cover_url must be an https URL of at most 2048 bytes"}
		}
		fields["cover_url"] = *p.CoverURL
	}
	dep, ret := j.DepartureDate, j.ReturnDate
	if p.DepartureDate != nil {
		dep = *p.DepartureDate
		fields["departure_date"] = dep
	}
	if p.ReturnDate != nil {
		ret = *p.ReturnDate
		fields["return_date"] = ret
	}
	if ret.Before(dep) {
		return nil, &ValidationError{Message: "return_date must not be before departure_date"}
	}
	if p.EditorsCanInvite != nil {
		fields["editors_can_invite"] = *p.EditorsCanInvite
	}

	if len(fields) > 0 {
		if err := s.repo.UpdateJourneyFields(ctx, j.ID, fields); err != nil {
			return nil, err
		}
	}
	updated, err := s.journeys.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrNotFound
	}
	return updated, nil
}

func (s *ItineraryService) AddDay(ctx context.Context, userID, slug string, in DayInput) (*models.ItineraryDay, error) {
	a, err := s.editable(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	if in.Date == nil || blank(*in.Date) {
		return nil, &ValidationError{Message: "date is required"}
	}
	if in.DateISO == nil || in.DateISO.IsZero() {
		return nil, &ValidationError{Message: "date_iso is required"}
	}
	d := &models.ItineraryDay{JourneyID: a.Journey.ID, Date: *in.Date, DateISO: *in.DateISO}
	if in.Title != nil {
		d.Title = *in.Title
	}
	if err := s.repo.CreateDay(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *ItineraryService) UpdateDay(ctx context.Context, userID, slug, dayID string, in DayInput) (*models.ItineraryDay, error) {
	a, err := s.editable(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	d, err := s.repo.GetDay(ctx, a.Journey.ID, dayID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, ErrDayNotFound
	}
	if in.Date != nil {
		if blank(*in.Date) {
			return nil, &ValidationError{Message: "date must not be empty"}
		}
		d.Date = *in.Date
	}
	if in.DateISO != nil {
		if in.DateISO.IsZero() {
			return nil, &ValidationError{Message: "date_iso must not be empty"}
		}
		d.DateISO = *in.DateISO
	}
	if in.Title != nil {
		d.Title = *in.Title
	}
	if err := s.repo.UpdateDay(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *ItineraryService) DeleteDay(ctx context.Context, userID, slug, dayID string) error {
	a, err := s.editable(ctx, userID, slug)
	if err != nil {
		return err
	}
	d, err := s.repo.GetDay(ctx, a.Journey.ID, dayID)
	if err != nil {
		return err
	}
	if d == nil {
		return ErrDayNotFound
	}
	return s.repo.DeleteDay(ctx, d)
}

func (s *ItineraryService) AddPlan(ctx context.Context, userID, slug, dayID string, in PlanInput) (*models.ActivityPlan, error) {
	a, err := s.editable(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	d, err := s.repo.GetDay(ctx, a.Journey.ID, dayID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, ErrDayNotFound
	}
	if in.Description == nil || blank(*in.Description) {
		return nil, &ValidationError{Message: "description is required"}
	}
	max, err := s.repo.MaxPosition(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	p := &models.ActivityPlan{ItineraryDayID: d.ID, Position: max + 1}
	applyPlanInput(p, in)
	if err := s.repo.CreatePlan(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func applyPlanInput(p *models.ActivityPlan, in PlanInput) {
	if in.Time != nil {
		p.Time = *in.Time
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	if in.Emoji != nil {
		p.Emoji = *in.Emoji
	}
	if in.MapURL != nil {
		p.MapURL = *in.MapURL
	}
}

func (s *ItineraryService) UpdatePlan(ctx context.Context, userID, slug, planID string, in PlanInput) (*models.ActivityPlan, error) {
	a, err := s.editable(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.GetPlan(ctx, a.Journey.ID, planID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPlanNotFound
	}
	if in.Description != nil && blank(*in.Description) {
		return nil, &ValidationError{Message: "description must not be empty"}
	}
	if in.DayID != nil && *in.DayID != p.ItineraryDayID {
		target, err := s.repo.GetDay(ctx, a.Journey.ID, *in.DayID)
		if err != nil {
			return nil, err
		}
		if target == nil {
			return nil, ErrDayNotFound
		}
		max, err := s.repo.MaxPosition(ctx, target.ID)
		if err != nil {
			return nil, err
		}
		p.ItineraryDayID = target.ID
		p.Position = max + 1
	}
	applyPlanInput(p, in)
	if err := s.repo.UpdatePlan(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *ItineraryService) DeletePlan(ctx context.Context, userID, slug, planID string) error {
	a, err := s.editable(ctx, userID, slug)
	if err != nil {
		return err
	}
	p, err := s.repo.GetPlan(ctx, a.Journey.ID, planID)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrPlanNotFound
	}
	return s.repo.DeletePlan(ctx, p.ID)
}

func (s *ItineraryService) ReorderPlans(ctx context.Context, userID, slug, dayID string, planIDs []string) error {
	a, err := s.editable(ctx, userID, slug)
	if err != nil {
		return err
	}
	d, err := s.repo.GetDay(ctx, a.Journey.ID, dayID)
	if err != nil {
		return err
	}
	if d == nil {
		return ErrDayNotFound
	}
	existing, err := s.repo.ListPlanIDs(ctx, d.ID)
	if err != nil {
		return err
	}
	if !sameSet(existing, planIDs) {
		return &ValidationError{Message: "plan_ids must contain exactly the plans of this day"}
	}
	return s.repo.SetPositions(ctx, d.ID, planIDs)
}

// sameSet reports whether a and b hold the same IDs with no duplicates.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] || (i > 0 && x[i] == x[i-1]) {
			return false
		}
	}
	return true
}
