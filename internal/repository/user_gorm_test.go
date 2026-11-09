package repository

import (
	"context"
	"testing"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRepository_UniqueEmailAndUsername(t *testing.T) {
	repo := NewUserRepository(testutil.NewTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &models.User{Email: "a@x.com", Username: "alice"}))
	assert.Error(t, repo.Create(ctx, &models.User{Email: "a@x.com", Username: "other"}))
	assert.Error(t, repo.Create(ctx, &models.User{Email: "b@x.com", Username: "alice"}))
	// multiple users without Google (nil GoogleSub) must not collide
	require.NoError(t, repo.Create(ctx, &models.User{Email: "c@x.com", Username: "carol"}))
}

func TestUserRepository_GetBy(t *testing.T) {
	repo := NewUserRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	sub := "g-123"
	u := &models.User{Email: "a@x.com", Username: "alice", GoogleSub: &sub}
	require.NoError(t, repo.Create(ctx, u))
	assert.NotEmpty(t, u.ID)

	got, err := repo.GetByEmail(ctx, "a@x.com")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, u.ID, got.ID)

	got, err = repo.GetByGoogleSub(ctx, "g-123")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, u.ID, got.ID)

	got, err = repo.GetByUsername(ctx, "alice")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, u.ID, got.ID)

	got, err = repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	for _, f := range []func() (*models.User, error){
		func() (*models.User, error) { return repo.GetByID(ctx, "nope") },
		func() (*models.User, error) { return repo.GetByEmail(ctx, "no@x.com") },
		func() (*models.User, error) { return repo.GetByGoogleSub(ctx, "nope") },
		func() (*models.User, error) { return repo.GetByUsername(ctx, "nope") },
	} {
		got, err := f()
		assert.NoError(t, err)
		assert.Nil(t, got)
	}
}

func TestUserRepository_Update(t *testing.T) {
	repo := NewUserRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	u := &models.User{Email: "a@x.com", Username: "alice"}
	require.NoError(t, repo.Create(ctx, u))
	u.Bio = "hello"
	require.NoError(t, repo.Update(ctx, u))
	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "hello", got.Bio)
}

func TestUserRepository_GetByIDs(t *testing.T) {
	repo := NewUserRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	for _, n := range []string{"a", "b", "c"} {
		require.NoError(t, repo.Create(ctx, &models.User{ID: n, Email: n + "@x.com", Username: n, DisplayName: n}))
	}
	got, err := repo.GetByIDs(ctx, []string{"a", "c", "missing"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	none, err := repo.GetByIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
