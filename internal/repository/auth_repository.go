package repository

import (
	"context"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
)

type AuthRepository interface {
	CreateSession(ctx context.Context, s *models.Session) error
	GetSessionByHash(ctx context.Context, hash string) (*models.Session, error)
	DeleteSessionByHash(ctx context.Context, hash string) error
	CreateLoginToken(ctx context.Context, t *models.LoginToken) error
	GetLoginTokenByHash(ctx context.Context, hash string) (*models.LoginToken, error)
	// MarkLoginTokenUsed sets used_at only on a row whose used_at IS NULL; returns false if
	// the token was already used (guards against the link being opened twice concurrently).
	MarkLoginTokenUsed(ctx context.Context, id string, at time.Time) (bool, error)
	CountLoginTokensSince(ctx context.Context, email string, since time.Time) (int64, error)
	// CountAllLoginTokensSince counts login links sent to anyone since the given time.
	CountAllLoginTokensSince(ctx context.Context, since time.Time) (int64, error)
	// DeleteExpired removes sessions that expired before now and login tokens created
	// before tokensBefore; it returns how many of each were removed.
	DeleteExpired(ctx context.Context, now, tokensBefore time.Time) (sessions, tokens int64, err error)
}
