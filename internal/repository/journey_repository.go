package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/kpkipper/journey-service/internal/models"
)

type JourneyRepository interface {
	Create(ctx context.Context, journey *models.Journey) error
	List(ctx context.Context) ([]models.Journey, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Journey, error)
	Update(ctx context.Context, journey *models.Journey) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteAll(ctx context.Context) error
}
