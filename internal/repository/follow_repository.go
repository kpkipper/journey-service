package repository

import "context"

type FollowRepository interface {
	// Follow is idempotent: following twice is not an error.
	Follow(ctx context.Context, followerID, followeeID string) error
	Unfollow(ctx context.Context, followerID, followeeID string) error
	IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error)
	CountFollowers(ctx context.Context, userID string) (int64, error)
	CountFollowing(ctx context.Context, userID string) (int64, error)
	ListFolloweeIDs(ctx context.Context, followerID string) ([]string, error)
}
