package repository

import (
	"context"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
)

type ReportRepository interface {
	Create(ctx context.Context, r *models.Report) error
	// FindOpen returns the reporter's open report on the target, or nil, nil.
	FindOpen(ctx context.Context, reporterID, targetType, targetID string) (*models.Report, error)
	// CountSince counts the reporter's reports created at or after since.
	CountSince(ctx context.Context, reporterID string, since time.Time) (int64, error)
	// ListOpen returns open reports, oldest first.
	ListOpen(ctx context.Context) ([]models.Report, error)
	// Get returns nil, nil when the report does not exist.
	Get(ctx context.Context, id string) (*models.Report, error)
	// Resolve marks an open report resolved; resolving a resolved one is a no-op.
	Resolve(ctx context.Context, id string, at time.Time) error
}
