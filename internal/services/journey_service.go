package services

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

var ErrNotFound = errors.New("journey not found")

type JourneyService struct {
	repo repository.JourneyRepository
}

func NewJourneyService(repo repository.JourneyRepository) *JourneyService {
	return &JourneyService{repo: repo}
}

func (s *JourneyService) Create(ctx context.Context, journey *models.Journey) error {
	return s.repo.Create(ctx, journey)
}

func (s *JourneyService) List(ctx context.Context) ([]models.Journey, error) {
	return s.repo.List(ctx)
}

func (s *JourneyService) GetByID(ctx context.Context, id uuid.UUID) (*models.Journey, error) {
	journey, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if journey == nil {
		return nil, ErrNotFound
	}
	return journey, nil
}

func (s *JourneyService) Update(ctx context.Context, id uuid.UUID, updated *models.Journey) (*models.Journey, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}

	updated.ID = id
	updated.CreatedAt = existing.CreatedAt

	if err := s.repo.Update(ctx, updated); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}

func (s *JourneyService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}
