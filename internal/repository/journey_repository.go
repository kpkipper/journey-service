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
	// ListByOwner returns the journeys owned by ownerID (no itinerary preloaded).
	ListByOwner(ctx context.Context, ownerID string) ([]models.Journey, error)
	// ListByIDs returns the journeys with the given IDs (no itinerary preloaded).
	ListByIDs(ctx context.Context, ids []string) ([]models.Journey, error)
	// ListByOwners returns up to limit journeys of the given owners, newest created first (no itinerary preloaded).
	ListByOwners(ctx context.Context, ownerIDs []string, limit int) ([]models.Journey, error)
	Update(ctx context.Context, journey *models.Journey) error
	// SwapCoverKey sets cover_key to next (and clears cover_url) only while it is
	// still prev, so two concurrent cover changes cannot both win. It reports
	// whether the row changed.
	SwapCoverKey(ctx context.Context, journeyID, prev, next string) (bool, error)
	// CoverKeyOf returns the cover key of a journey, deleted or not ("" if none).
	CoverKeyOf(ctx context.Context, journeyID string) (string, error)
	Delete(ctx context.Context, slug string) error
}
