package repository

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
)

type JourneyRepository interface {
	Create(ctx context.Context, journey *models.Journey) error
	List(ctx context.Context) ([]models.Journey, error)
	GetBySlug(ctx context.Context, slug string) (*models.Journey, error)
	// GetByID returns nil, nil when the journey does not exist.
	GetByID(ctx context.Context, id string) (*models.Journey, error)
	Update(ctx context.Context, journey *models.Journey) error
	Delete(ctx context.Context, slug string) error
}
