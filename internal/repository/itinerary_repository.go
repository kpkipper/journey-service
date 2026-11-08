package repository

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
)

// ItineraryRepository edits days and plans. Day/plan lookups are always scoped
// to a journey; they return nil, nil when the row is not in that journey.
type ItineraryRepository interface {
	GetDay(ctx context.Context, journeyID, dayID string) (*models.ItineraryDay, error)
	CreateDay(ctx context.Context, d *models.ItineraryDay) error
	UpdateDay(ctx context.Context, d *models.ItineraryDay) error
	DeleteDay(ctx context.Context, d *models.ItineraryDay) error
	GetPlan(ctx context.Context, journeyID, planID string) (*models.ActivityPlan, error)
	CreatePlan(ctx context.Context, p *models.ActivityPlan) error
	UpdatePlan(ctx context.Context, p *models.ActivityPlan) error
	DeletePlan(ctx context.Context, planID string) error
	ListPlanIDs(ctx context.Context, dayID string) ([]string, error)
	// MaxPosition returns -1 when the day has no plans.
	MaxPosition(ctx context.Context, dayID string) (int, error)
	// SetPositions sets position = index for each id, in one transaction.
	SetPositions(ctx context.Context, dayID string, ids []string) error
	// UpdateJourneyFields updates only the given columns.
	UpdateJourneyFields(ctx context.Context, journeyID string, fields map[string]any) error
}
