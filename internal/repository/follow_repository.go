package repository

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
)

type FollowRepository interface {
	// Follow is idempotent: following twice is not an error.
	Follow(ctx context.Context, followerID, followeeID string) error
	Unfollow(ctx context.Context, followerID, followeeID string) error
	IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error)
	CountFollowers(ctx context.Context, userID string) (int64, error)
	CountFollowing(ctx context.Context, userID string) (int64, error)
	ListFolloweeIDs(ctx context.Context, followerID string) ([]string, error)

	// Request is idempotent and keeps the original created_at.
	Request(ctx context.Context, requesterID, targetID string) error
	CancelRequest(ctx context.Context, requesterID, targetID string) error
	HasRequest(ctx context.Context, requesterID, targetID string) (bool, error)
	CountOutgoingRequests(ctx context.Context, requesterID string) (int64, error)
	// AcceptRequest turns a pending request into a follow; false if none existed.
	AcceptRequest(ctx context.Context, requesterID, targetID string) (bool, error)
	// AcceptAllRequests follows every requester not in skip, then clears all requests to targetID.
	AcceptAllRequests(ctx context.Context, targetID string, skip map[string]bool) error
	ListRequests(ctx context.Context, targetID string, limit int) ([]models.FollowEntry, error)
	CountRequests(ctx context.Context, targetID string) (int64, error)
	ListFollowers(ctx context.Context, userID string, limit int) ([]models.FollowEntry, error)
}
