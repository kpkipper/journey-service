package services

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
)

// Access describes what a user may do in one trip.
type Access struct {
	Journey *models.Journey
	Role    string // "owner" | "editor" | "traveler" | "" (not a member, or still invited)
}

type AccessService struct {
	journeys repository.JourneyRepository
	members  repository.MemberRepository
	follows  repository.FollowRepository // optional; nil means nobody follows anybody
	blocks   repository.BlockRepository  // optional; nil means no blocks
}

func NewAccessService(j repository.JourneyRepository, m repository.MemberRepository) *AccessService {
	return &AccessService{journeys: j, members: m}
}

// WithFollows lets Deny tell followers from strangers; without it everyone who
// is not a member counts as a stranger.
func (s *AccessService) WithFollows(f repository.FollowRepository) *AccessService {
	s.follows = f
	return s
}

// WithBlocks makes trips of a block-related owner invisible to non-members.
func (s *AccessService) WithBlocks(b repository.BlockRepository) *AccessService {
	s.blocks = b
	return s
}

// HiddenByBlock reports whether the trip must look missing to viewerID: the
// viewer is not a member, the trip has an owner, and the viewer and the owner
// blocked each other (either direction). Members keep access.
func (s *AccessService) HiddenByBlock(ctx context.Context, viewerID string, a *Access) (bool, error) {
	if s.blocks == nil || viewerID == "" || a.IsMember() || a.Journey.OwnerID == "" || a.Journey.OwnerID == viewerID {
		return false, nil
	}
	return s.blocks.Blocked(ctx, viewerID, a.Journey.OwnerID)
}

// Deny is the error for an actor who lacks the rights for a write. A non-member
// who may not even see the trip gets ErrNotFound (the trip must not reveal
// itself); anyone else gets ErrForbidden.
func (s *AccessService) Deny(ctx context.Context, a *Access, userID string) error {
	if a.IsMember() {
		return ErrForbidden
	}
	if hidden, err := s.HiddenByBlock(ctx, userID, a); err != nil {
		return err
	} else if hidden {
		return ErrNotFound
	}
	rel, err := relationOf(ctx, s.follows, userID, a.Journey, a.Role)
	if err != nil {
		return err
	}
	if !canSee(rel, a.Journey) {
		return ErrNotFound
	}
	return ErrForbidden
}

// Load resolves the user's role in the trip. userID may be empty (anonymous).
// Returns ErrNotFound when the trip does not exist.
func (s *AccessService) Load(ctx context.Context, slug, userID string) (*Access, error) {
	journey, err := s.journeys.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if journey == nil {
		return nil, ErrNotFound
	}
	return s.ForJourney(ctx, journey, userID)
}

// ForJourney resolves the user's role in an already-loaded trip. userID may be empty.
func (s *AccessService) ForJourney(ctx context.Context, journey *models.Journey, userID string) (*Access, error) {
	a := &Access{Journey: journey}
	switch {
	case journey.OwnerID != "" && journey.OwnerID == userID:
		a.Role = models.RoleOwner
	case userID != "":
		m, err := s.members.Get(ctx, journey.ID, userID)
		if err != nil {
			return nil, err
		}
		// Only editor/traveler are valid stored roles; owner comes solely from journeys.owner_id.
		if m != nil && m.Status == models.StatusJoined &&
			(m.Role == models.RoleEditor || m.Role == models.RoleTraveler) {
			a.Role = m.Role
		}
	}
	return a, nil
}

// legacy reports a trip without an owner: read-only for everyone.
func (a *Access) legacy() bool { return a.Journey.OwnerID == "" }

func (a *Access) IsOwner() bool  { return !a.legacy() && a.Role == models.RoleOwner }
func (a *Access) IsMember() bool { return a.Role != "" }

func (a *Access) CanEdit() bool {
	return !a.legacy() && (a.Role == models.RoleOwner || a.Role == models.RoleEditor)
}

func (a *Access) CanInvite() bool {
	if a.legacy() {
		return false
	}
	return a.Role == models.RoleOwner || (a.Role == models.RoleEditor && a.Journey.EditorsCanInvite)
}
