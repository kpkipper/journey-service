package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthRepository_Session(t *testing.T) {
	db := testutil.NewTestDB(t)
	users := NewUserRepository(db)
	repo := NewAuthRepository(db)
	ctx := context.Background()
	u := &models.User{Email: "a@x.com", Username: "alice"}
	require.NoError(t, users.Create(ctx, u))

	require.NoError(t, repo.CreateSession(ctx, &models.Session{UserID: u.ID, TokenHash: "h1", ExpiresAt: time.Now().Add(time.Hour)}))
	got, err := repo.GetSessionByHash(ctx, "h1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, u.ID, got.UserID)

	require.NoError(t, repo.DeleteSessionByHash(ctx, "h1"))
	got, err = repo.GetSessionByHash(ctx, "h1")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestAuthRepository_MarkLoginTokenUsed(t *testing.T) {
	repo := NewAuthRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	tok := &models.LoginToken{Email: "a@x.com", TokenHash: "t1", ExpiresAt: time.Now().Add(15 * time.Minute)}
	require.NoError(t, repo.CreateLoginToken(ctx, tok))

	got, err := repo.GetLoginTokenByHash(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.UsedAt)

	ok, err := repo.MarkLoginTokenUsed(ctx, tok.ID, time.Now())
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = repo.MarkLoginTokenUsed(ctx, tok.ID, time.Now())
	require.NoError(t, err)
	assert.False(t, ok)

	got, err = repo.GetLoginTokenByHash(ctx, "t1")
	require.NoError(t, err)
	assert.NotNil(t, got.UsedAt)

	got, err = repo.GetLoginTokenByHash(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestAuthRepository_CountLoginTokensSince(t *testing.T) {
	repo := NewAuthRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	now := time.Now()
	for i, tc := range []struct {
		email string
		at    time.Time
	}{
		{"a@x.com", now.Add(-2 * time.Hour)},
		{"a@x.com", now.Add(-30 * time.Minute)},
		{"a@x.com", now.Add(-1 * time.Minute)},
		{"b@x.com", now.Add(-1 * time.Minute)},
	} {
		require.NoError(t, repo.CreateLoginToken(ctx, &models.LoginToken{
			Email: tc.email, TokenHash: fmt.Sprintf("hash%d", i), ExpiresAt: now.Add(time.Hour), CreatedAt: tc.at,
		}))
	}
	n, err := repo.CountLoginTokensSince(ctx, "a@x.com", now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestAuthRepository_CountAllLoginTokensSince(t *testing.T) {
	repo := NewAuthRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	now := time.Now()
	for i, at := range []time.Time{now.Add(-2 * time.Hour), now.Add(-30 * time.Minute), now.Add(-time.Minute)} {
		require.NoError(t, repo.CreateLoginToken(ctx, &models.LoginToken{
			Email: fmt.Sprintf("u%d@x.com", i), TokenHash: fmt.Sprintf("all%d", i), ExpiresAt: now.Add(time.Hour), CreatedAt: at,
		}))
	}
	n, err := repo.CountAllLoginTokensSince(ctx, now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestAuthRepository_DeleteExpired(t *testing.T) {
	db := testutil.NewTestDB(t)
	users := NewUserRepository(db)
	repo := NewAuthRepository(db)
	ctx := context.Background()
	now := time.Now()
	u := &models.User{Email: "a@x.com", Username: "alice"}
	require.NoError(t, users.Create(ctx, u))

	require.NoError(t, repo.CreateSession(ctx, &models.Session{UserID: u.ID, TokenHash: "old", ExpiresAt: now.Add(-time.Minute)}))
	require.NoError(t, repo.CreateSession(ctx, &models.Session{UserID: u.ID, TokenHash: "live", ExpiresAt: now.Add(time.Hour)}))
	// Login tokens are kept for a day: the per-email and total limits count the last hour.
	require.NoError(t, repo.CreateLoginToken(ctx, &models.LoginToken{Email: "a@x.com", TokenHash: "t-old", ExpiresAt: now.Add(-24 * time.Hour), CreatedAt: now.Add(-25 * time.Hour)}))
	require.NoError(t, repo.CreateLoginToken(ctx, &models.LoginToken{Email: "a@x.com", TokenHash: "t-recent", ExpiresAt: now.Add(-time.Hour), CreatedAt: now.Add(-2 * time.Hour)}))

	sessions, tokens, err := repo.DeleteExpired(ctx, now, now.Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), sessions)
	assert.Equal(t, int64(1), tokens)

	got, err := repo.GetSessionByHash(ctx, "live")
	require.NoError(t, err)
	assert.NotNil(t, got)
	got, err = repo.GetSessionByHash(ctx, "old")
	require.NoError(t, err)
	assert.Nil(t, got)
	lt, err := repo.GetLoginTokenByHash(ctx, "t-recent")
	require.NoError(t, err)
	assert.NotNil(t, lt)
	lt, err = repo.GetLoginTokenByHash(ctx, "t-old")
	require.NoError(t, err)
	assert.Nil(t, lt)
}
