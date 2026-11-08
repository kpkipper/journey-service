package repository

import (
	"context"
	"testing"

	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFollowRepository_FollowIsIdempotent(t *testing.T) {
	repo := NewFollowRepository(testutil.NewTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.Follow(ctx, "u1", "u2"))
	require.NoError(t, repo.Follow(ctx, "u1", "u2"))

	n, err := repo.CountFollowers(ctx, "u2")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

func TestFollowRepository_Unfollow(t *testing.T) {
	repo := NewFollowRepository(testutil.NewTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.Follow(ctx, "u1", "u2"))
	ok, err := repo.IsFollowing(ctx, "u1", "u2")
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, repo.Unfollow(ctx, "u1", "u2"))
	ok, err = repo.IsFollowing(ctx, "u1", "u2")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFollowRepository_CountsAndList(t *testing.T) {
	repo := NewFollowRepository(testutil.NewTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.Follow(ctx, "u1", "u2"))
	require.NoError(t, repo.Follow(ctx, "u1", "u3"))
	require.NoError(t, repo.Follow(ctx, "u3", "u2"))

	followers, err := repo.CountFollowers(ctx, "u2")
	require.NoError(t, err)
	assert.Equal(t, int64(2), followers)

	following, err := repo.CountFollowing(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), following)

	ids, err := repo.ListFolloweeIDs(ctx, "u1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"u2", "u3"}, ids)

	none, err := repo.ListFolloweeIDs(ctx, "u9")
	require.NoError(t, err)
	assert.Empty(t, none)
}
