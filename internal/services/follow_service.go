package services

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

// FollowService: following is one-way; targets who approve followers get requests.
type FollowService struct {
	users   repository.UserRepository
	follows repository.FollowRepository
	blocks  repository.BlockRepository // optional
}

func NewFollowService(users repository.UserRepository, follows repository.FollowRepository) *FollowService {
	return &FollowService{users: users, follows: follows}
}

// WithBlocks makes Follow refuse block-related users (as if they didn't exist).
func (s *FollowService) WithBlocks(b repository.BlockRepository) *FollowService {
	s.blocks = b
	return s
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

const (
	maxOutgoingRequests = 200
	maxRequestsListed   = 100
	maxFollowersListed  = 200
)

// FollowState is the viewer's relation to a user after a follow or unfollow.
type FollowState struct {
	IsFollowing bool  `json:"is_following"`
	Requested   bool  `json:"requested"`
	Followers   int64 `json:"followers"`
}

// related returns the ids block-related to userID (nil when blocks aren't wired).
func (s *FollowService) related(ctx context.Context, userID string) (map[string]bool, error) {
	if s.blocks == nil {
		return nil, nil
	}
	return s.blocks.RelatedIDs(ctx, userID)
}

func (s *FollowService) state(ctx context.Context, targetID string, st FollowState) (FollowState, error) {
	n, err := s.follows.CountFollowers(ctx, targetID)
	if err != nil {
		return FollowState{}, err
	}
	st.Followers = n
	return st, nil
}

// Follow is idempotent. Following yourself is a ValidationError. When the
// target approves followers, it creates a pending request instead.
func (s *FollowService) Follow(ctx context.Context, followerID, username string) (FollowState, error) {
	u, err := s.user(ctx, username)
	if err != nil {
		return FollowState{}, err
	}
	if u.ID == followerID {
		return FollowState{}, &ValidationError{Message: "cannot follow yourself"}
	}
	if err := s.notFoundIfBlocked(ctx, followerID, u.ID); err != nil {
		return FollowState{}, err
	}
	already, err := s.follows.IsFollowing(ctx, followerID, u.ID)
	if err != nil {
		return FollowState{}, err
	}
	if already {
		return s.state(ctx, u.ID, FollowState{IsFollowing: true})
	}
	if !u.FollowApproval {
		if err := s.follows.Follow(ctx, followerID, u.ID); err != nil {
			return FollowState{}, err
		}
		if err := s.follows.CancelRequest(ctx, followerID, u.ID); err != nil { // stale request
			return FollowState{}, err
		}
		return s.state(ctx, u.ID, FollowState{IsFollowing: true})
	}
	has, err := s.follows.HasRequest(ctx, followerID, u.ID)
	if err != nil {
		return FollowState{}, err
	}
	if !has {
		n, err := s.follows.CountOutgoingRequests(ctx, followerID)
		if err != nil {
			return FollowState{}, err
		}
		if n >= maxOutgoingRequests {
			return FollowState{}, ErrTooManyRequests
		}
		if err := s.follows.Request(ctx, followerID, u.ID); err != nil {
			return FollowState{}, err
		}
	}
	return s.state(ctx, u.ID, FollowState{Requested: true})
}

// Unfollow is idempotent; it also cancels a pending request. For a
// block-related user it still deletes the rows, then returns ErrUserNotFound.
func (s *FollowService) Unfollow(ctx context.Context, followerID, username string) (FollowState, error) {
	u, err := s.user(ctx, username)
	if err != nil {
		return FollowState{}, err
	}
	if err := s.follows.Unfollow(ctx, followerID, u.ID); err != nil {
		return FollowState{}, err
	}
	if err := s.follows.CancelRequest(ctx, followerID, u.ID); err != nil {
		return FollowState{}, err
	}
	if err := s.notFoundIfBlocked(ctx, followerID, u.ID); err != nil {
		return FollowState{}, err
	}
	return s.state(ctx, u.ID, FollowState{})
}

// notFoundIfBlocked returns ErrUserNotFound when a and b are block-related.
// Callers that clean up rows do so first, then report the user as missing.
func (s *FollowService) notFoundIfBlocked(ctx context.Context, a, b string) error {
	if s.blocks == nil {
		return nil
	}
	blocked, err := s.blocks.Blocked(ctx, a, b)
	if err != nil {
		return err
	}
	if blocked {
		return ErrUserNotFound
	}
	return nil
}

func (s *FollowService) filter(ctx context.Context, userID string, in []models.FollowEntry) ([]models.FollowEntry, error) {
	rel, err := s.related(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]models.FollowEntry, 0, len(in))
	for _, e := range in {
		if rel[e.UserID] {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// Requests lists pending requests to userID, newest first, minus block-related users.
func (s *FollowService) Requests(ctx context.Context, userID string) ([]models.FollowEntry, error) {
	in, err := s.follows.ListRequests(ctx, userID, maxRequestsListed)
	if err != nil {
		return nil, err
	}
	return s.filter(ctx, userID, in)
}

// Followers lists userID's followers, newest first, minus block-related users.
func (s *FollowService) Followers(ctx context.Context, userID string) ([]models.FollowEntry, error) {
	in, err := s.follows.ListFollowers(ctx, userID, maxFollowersListed)
	if err != nil {
		return nil, err
	}
	return s.filter(ctx, userID, in)
}

// RequestCount is the number of pending requests to userID from users who
// are not block-related (the same set Requests lists, before its cap).
func (s *FollowService) RequestCount(ctx context.Context, userID string) (int64, error) {
	return s.follows.CountRequests(ctx, userID)
}

// Accept turns requesterUsername's pending request into a follow.
func (s *FollowService) Accept(ctx context.Context, userID, requesterUsername string) error {
	u, err := s.user(ctx, requesterUsername)
	if err != nil {
		return err
	}
	if err := s.notFoundIfBlocked(ctx, userID, u.ID); err != nil {
		return err
	}
	ok, err := s.follows.AcceptRequest(ctx, u.ID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrUserNotFound
	}
	return nil
}

// Decline removes a pending request. Idempotent. For a block-related user
// it still deletes the row, then returns ErrUserNotFound.
func (s *FollowService) Decline(ctx context.Context, userID, requesterUsername string) error {
	u, err := s.user(ctx, requesterUsername)
	if err != nil {
		return err
	}
	if err := s.follows.CancelRequest(ctx, u.ID, userID); err != nil {
		return err
	}
	return s.notFoundIfBlocked(ctx, userID, u.ID)
}

// RemoveFollower deletes the follow followerUsername -> userID. Idempotent.
// For a block-related user it still deletes the row, then returns ErrUserNotFound.
func (s *FollowService) RemoveFollower(ctx context.Context, userID, followerUsername string) error {
	u, err := s.user(ctx, followerUsername)
	if err != nil {
		return err
	}
	if err := s.follows.Unfollow(ctx, u.ID, userID); err != nil {
		return err
	}
	return s.notFoundIfBlocked(ctx, userID, u.ID)
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
