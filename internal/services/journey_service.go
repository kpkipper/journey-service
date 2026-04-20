package services

import (
	"context"
	"errors"

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

func (s *JourneyService) Update(ctx context.Context, slug string, updated *models.Journey) error {
	existing, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrNotFound
	}

	updated.ID = existing.ID
	updated.Slug = existing.Slug
	updated.CreatedAt = existing.CreatedAt

	return s.repo.Update(ctx, updated)
}

func (s *JourneyService) Delete(ctx context.Context, slug string) error {
	return s.repo.Delete(ctx, slug)
}

func (s *JourneyService) DeleteAll(ctx context.Context) error {
	return s.repo.DeleteAll(ctx)
}
