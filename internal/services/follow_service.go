package services

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

// FollowService: following is instant (no approval) and one-way.
type FollowService struct {
	users   repository.UserRepository
	follows repository.FollowRepository
}

func NewFollowService(users repository.UserRepository, follows repository.FollowRepository) *FollowService {
	return &FollowService{users: users, follows: follows}
}

func (s *FollowService) user(ctx context.Context, username string) (*models.User, error) {
	u, err := s.users.GetByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// Follow is idempotent. Following yourself is a ValidationError.
func (s *FollowService) Follow(ctx context.Context, followerID, username string) error {
	u, err := s.user(ctx, username)
	if err != nil {
		return err
	}
	if u.ID == followerID {
		return &ValidationError{Message: "cannot follow yourself"}
	}
	return s.follows.Follow(ctx, followerID, u.ID)
}

// Unfollow is idempotent.
func (s *FollowService) Unfollow(ctx context.Context, followerID, username string) error {
	u, err := s.user(ctx, username)
	if err != nil {
		return err
	}
	return s.follows.Unfollow(ctx, followerID, u.ID)
}

// Counts returns how many people follow the user and how many the user follows.
func (s *FollowService) Counts(ctx context.Context, username string) (followers, following int64, err error) {
	u, err := s.user(ctx, username)
	if err != nil {
		return 0, 0, err
	}
	return countsOf(ctx, s.follows, u.ID)
}

func countsOf(ctx context.Context, follows repository.FollowRepository, userID string) (followers, following int64, err error) {
	if followers, err = follows.CountFollowers(ctx, userID); err != nil {
		return 0, 0, err
	}
	if following, err = follows.CountFollowing(ctx, userID); err != nil {
		return 0, 0, err
	}
	return followers, following, nil
}
