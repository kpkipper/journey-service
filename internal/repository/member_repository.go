package repository

import (
	"context"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
)

type MemberRepository interface {
	Create(ctx context.Context, m *models.JourneyMember) error
	// Get returns nil, nil when the user has no row in the journey.
	Get(ctx context.Context, journeyID, userID string) (*models.JourneyMember, error)
	GetByID(ctx context.Context, id string) (*models.JourneyMember, error)
	Update(ctx context.Context, m *models.JourneyMember) error
	Delete(ctx context.Context, id string) error
	// ListByJourney joins users; joined members first, then by created_at.
	ListByJourney(ctx context.Context, journeyID string) ([]models.MemberView, error)
	ListInvitesForUser(ctx context.Context, userID string) ([]models.JourneyMember, error)
	ListJourneyIDsForUser(ctx context.Context, userID string) ([]string, error)
	// CountActive counts invited + joined rows.
	CountActive(ctx context.Context, journeyID string) (int64, error)
	CreateInviteLink(ctx context.Context, l *models.InviteLink) error
	GetInviteLinkByHash(ctx context.Context, hash string) (*models.InviteLink, error)
	// GetInviteLink is scoped to journeyID so another journey's link is not found.
	GetInviteLink(ctx context.Context, journeyID, id string) (*models.InviteLink, error)
	RevokeInviteLink(ctx context.Context, id string, at time.Time) error
	// ListInviteLinks returns non-revoked links (possibly expired).
	ListInviteLinks(ctx context.Context, journeyID string) ([]models.InviteLink, error)
}
