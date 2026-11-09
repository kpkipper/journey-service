package repository

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
)

type BlockRepository interface {
	// Block is idempotent: blocking twice is not an error.
	Block(ctx context.Context, blockerID, blockedID string) error
	// BlockAndUnfollow inserts the block and deletes follows in both
	// directions in one transaction.
	BlockAndUnfollow(ctx context.Context, blockerID, blockedID string) error
	Unblock(ctx context.Context, blockerID, blockedID string) error
	// Blocked reports whether either user blocked the other.
	Blocked(ctx context.Context, a, b string) (bool, error)
	// IsBlocking reports whether blockerID blocked blockedID (one direction).
	IsBlocking(ctx context.Context, blockerID, blockedID string) (bool, error)
	// ListBlocked returns the users blockerID blocked, newest first.
	ListBlocked(ctx context.Context, blockerID string) ([]models.PublicProfile, error)
	// RelatedIDs returns everyone block-related to userID, in either direction.
	RelatedIDs(ctx context.Context, userID string) (map[string]bool, error)
}
