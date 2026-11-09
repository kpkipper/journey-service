package repository

import (
	"context"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
)

type MomentRepository interface {
	Create(ctx context.Context, m *models.Moment) error
	// Get returns nil, nil when the moment does not exist.
	Get(ctx context.Context, id string) (*models.Moment, error)
	// MarkReady finalizes a pending moment; gorm.ErrRecordNotFound when it is not pending.
	MarkReady(ctx context.Context, m *models.Moment) error
	// MarkRejected moves a pending moment to rejected (pending only; else gorm.ErrRecordNotFound).
	MarkRejected(ctx context.Context, id string) error
	// DeleteRejectedBefore hard-deletes rejected rows created before t.
	DeleteRejectedBefore(ctx context.Context, t time.Time) (int64, error)
	UpdateCaption(ctx context.Context, id, caption string) error
	Delete(ctx context.Context, id string) error
	ListReadyByJourney(ctx context.Context, journeyID string) ([]models.Moment, error)
	ListByUploader(ctx context.Context, userID string) ([]models.Moment, error)
	CountByJourney(ctx context.Context, journeyID string) (int64, error)
	CountByDay(ctx context.Context, dayID string) (int64, error)
	CountPendingByUploader(ctx context.Context, userID string) (int64, error)
	CountStartedSince(ctx context.Context, userID string, since time.Time) (int64, error)
	DetachPlan(ctx context.Context, planID string) error
	ListByJourney(ctx context.Context, journeyID string) ([]models.Moment, error)
	ListPendingBefore(ctx context.Context, t time.Time) ([]models.Moment, error)
}
