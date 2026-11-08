package services

import (
	"context"
	"errors"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

var ErrNotFound = errors.New("journey not found")
var ErrForbidden = errors.New("forbidden")

type JourneyService struct {
	repo repository.JourneyRepository
}

func NewJourneyService(repo repository.JourneyRepository) *JourneyService {
	return &JourneyService{repo: repo}
}

func (s *JourneyService) Create(ctx context.Context, ownerID string, journey *models.Journey) error {
	journey.OwnerID = ownerID
	return s.repo.Create(ctx, journey)
}

func (s *JourneyService) List(ctx context.Context) ([]models.Journey, error) {
	return s.repo.List(ctx)
}

func (s *JourneyService) GetBySlug(ctx context.Context, slug string) (*models.Journey, error) {
	journey, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if journey == nil {
		return nil, ErrNotFound
	}
	return journey, nil
}

// owned loads the journey and checks userID owns it. Journeys with an empty
// OwnerID (legacy) are owned by nobody.
func (s *JourneyService) owned(ctx context.Context, userID, slug string) (*models.Journey, error) {
	existing, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if existing.OwnerID == "" || existing.OwnerID != userID {
		return nil, ErrForbidden
	}
	return existing, nil
}

func (s *JourneyService) Update(ctx context.Context, userID, slug string, updated *models.Journey) error {
	existing, err := s.owned(ctx, userID, slug)
	if err != nil {
		return err
	}

	updated.ID = existing.ID
	updated.Slug = existing.Slug
	updated.CreatedAt = existing.CreatedAt
	updated.OwnerID = existing.OwnerID

	return s.repo.Update(ctx, updated)
}

func (s *JourneyService) Delete(ctx context.Context, userID, slug string) error {
	if _, err := s.owned(ctx, userID, slug); err != nil {
		return err
	}
	return s.repo.Delete(ctx, slug)
}
