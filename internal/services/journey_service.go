package services

import (
	"context"
	"errors"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/pkg/logger"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("journey not found")
var ErrForbidden = errors.New("forbidden")

type JourneyService struct {
	repo   repository.JourneyRepository
	access *AccessService

	moments  repository.MomentRepository
	onDelete func(ctx context.Context, journeyID string) error
}

// WithMoments makes full updates fail while the trip has moments and runs
// onDelete (best effort, errors only logged) after a trip is deleted.
func (s *JourneyService) WithMoments(repo repository.MomentRepository, onDelete func(ctx context.Context, journeyID string) error) *JourneyService {
	s.moments, s.onDelete = repo, onDelete
	return s
}

func NewJourneyService(repo repository.JourneyRepository, access *AccessService) *JourneyService {
	return &JourneyService{repo: repo, access: access}
}

func (s *JourneyService) Create(ctx context.Context, ownerID string, journey *models.Journey) error {
	if err := validateCover(journey); err != nil {
		return err
	}
	if err := validateJourney(journey); err != nil {
		return err
	}
	if journey.Visibility == "" {
		journey.Visibility = models.VisibilityFollowers
	}
	if !validVisibility(journey.Visibility) {
		return errBadVisibility
	}
	if journey.Timezone == "" {
		journey.Timezone = "Asia/Bangkok"
	}
	if !validTimezone(journey.Timezone) {
		return errBadTimezone
	}
	journey.OwnerID = ownerID
	journey.ID = ""
	journey.DeletedAt = gorm.DeletedAt{}
	resetChildren(journey)
	return s.repo.Create(ctx, journey)
}

// resetChildren drops any client-supplied day/plan IDs (so fresh ones are
// generated and rows of other journeys can't be overwritten) and numbers
// plans by their order within each day.
func resetChildren(j *models.Journey) {
	for i := range j.ItineraryDays {
		d := &j.ItineraryDays[i]
		d.ID, d.JourneyID = "", ""
		for k := range d.Plans {
			d.Plans[k].ID, d.Plans[k].ItineraryDayID = "", ""
			d.Plans[k].Position = k
		}
	}
}

func (s *JourneyService) List(ctx context.Context) ([]models.Journey, error) {
	return s.repo.List(ctx)
}

// Update is the owner-only full replace of title, destination, country, dates and the
// day/plan tree. cover_url, editors_can_invite, visibility, is_draft, timezone and hide_live_status are PATCH-only and left untouched.
func (s *JourneyService) Update(ctx context.Context, userID, slug string, updated *models.Journey) error {
	a, err := s.access.Load(ctx, slug, userID)
	if err != nil {
		return err
	}
	if !a.IsOwner() {
		return s.access.Deny(ctx, a, userID)
	}
	if err := validateJourney(updated); err != nil {
		return err
	}
	if s.moments != nil {
		n, err := s.moments.CountByJourney(ctx, a.Journey.ID)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrTripHasMoments
		}
	}
	updated.ID = a.Journey.ID
	resetChildren(updated)

	if err := s.repo.Update(ctx, updated); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *JourneyService) Delete(ctx context.Context, userID, slug string) error {
	a, err := s.access.Load(ctx, slug, userID)
	if err != nil {
		return err
	}
	if !a.IsOwner() {
		return s.access.Deny(ctx, a, userID)
	}
	if err := s.repo.Delete(ctx, slug); err != nil {
		return err
	}
	if s.onDelete != nil {
		if err := s.onDelete(context.WithoutCancel(ctx), a.Journey.ID); err != nil {
			logger.Get().Error().Err(err).Str("journey", a.Journey.ID).Msg("delete moments of deleted trip failed")
		}
	}
	return nil
}
