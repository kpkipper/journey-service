package services

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

// BlockService: a block is one-directional but hides both users from each
// other, and removes follows in both directions.
type BlockService struct {
	users   repository.UserRepository
	blocks  repository.BlockRepository
	follows repository.FollowRepository
}

func NewBlockService(users repository.UserRepository, blocks repository.BlockRepository, follows repository.FollowRepository) *BlockService {
	return &BlockService{users: users, blocks: blocks, follows: follows}
}

func (s *BlockService) user(ctx context.Context, username string) (*models.User, error) {
	u, err := s.users.GetByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// Block is idempotent. Blocking yourself is a ValidationError.
func (s *BlockService) Block(ctx context.Context, blockerID, username string) error {
	u, err := s.user(ctx, username)
	if err != nil {
		return err
	}
	if u.ID == blockerID {
		return &ValidationError{Message: "cannot block yourself"}
	}
	return s.blocks.BlockAndUnfollow(ctx, blockerID, u.ID)
}

// Unblock is idempotent.
func (s *BlockService) Unblock(ctx context.Context, blockerID, username string) error {
	u, err := s.user(ctx, username)
	if err != nil {
		return err
	}
	return s.blocks.Unblock(ctx, blockerID, u.ID)
}

func (s *BlockService) List(ctx context.Context, blockerID string) ([]models.PublicProfile, error) {
	return s.blocks.ListBlocked(ctx, blockerID)
}
