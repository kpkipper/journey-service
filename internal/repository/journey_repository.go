package repository

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
)

type JourneyRepository interface {
	Create(ctx context.Context, journey *models.Journey) error
	List(ctx context.Context) ([]models.Journey, error)
	GetBySlug(ctx context.Context, slug string) (*models.Journey, error)
	Update(ctx context.Context, journey *models.Journey) error
	Delete(ctx context.Context, slug string) error
	DeleteAll(ctx context.Context) error
}
