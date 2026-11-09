package repository

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
)

type UserRepository interface {
	Create(ctx context.Context, u *models.User) error
	GetByID(ctx context.Context, id string) (*models.User, error)
	// GetByIDs returns the users that exist among ids, in one query.
	GetByIDs(ctx context.Context, ids []string) ([]models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByGoogleSub(ctx context.Context, sub string) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	Update(ctx context.Context, u *models.User) error
	// SetFollowApproval writes the flag explicitly so false is not skipped.
	SetFollowApproval(ctx context.Context, userID string, on bool) error
}
