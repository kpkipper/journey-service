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
}

func NewAccessService(j repository.JourneyRepository, m repository.MemberRepository) *AccessService {
	return &AccessService{journeys: j, members: m}
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
