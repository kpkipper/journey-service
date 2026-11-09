package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockRepository_BlockIdempotentAndBlockedBothWays(t *testing.T) {
	repo := NewBlockRepository(testutil.NewTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.Block(ctx, "u1", "u2"))
	require.NoError(t, repo.Block(ctx, "u1", "u2"))

	for _, p := range [][2]string{{"u1", "u2"}, {"u2", "u1"}} {
		ok, err := repo.Blocked(ctx, p[0], p[1])
		require.NoError(t, err)
		assert.True(t, ok)
	}
	ok, err := repo.Blocked(ctx, "u1", "u3")
	require.NoError(t, err)
	assert.False(t, ok)

	ok, _ = repo.IsBlocking(ctx, "u1", "u2")
	assert.True(t, ok)
	ok, _ = repo.IsBlocking(ctx, "u2", "u1")
	assert.False(t, ok)

	require.NoError(t, repo.Unblock(ctx, "u1", "u2"))
	require.NoError(t, repo.Unblock(ctx, "u1", "u2"))
	ok, _ = repo.Blocked(ctx, "u1", "u2")
	assert.False(t, ok)
}

func TestBlockRepository_RelatedIDs(t *testing.T) {
	repo := NewBlockRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	require.NoError(t, repo.Block(ctx, "u1", "u2"))
	require.NoError(t, repo.Block(ctx, "u3", "u1"))
	require.NoError(t, repo.Block(ctx, "u4", "u5"))

	ids, err := repo.RelatedIDs(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"u2": true, "u3": true}, ids)

	ids, err = repo.RelatedIDs(ctx, "nobody")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestBlockRepository_BlockAndUnfollow(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewBlockRepository(db)
	follows := NewFollowRepository(db)
	ctx := context.Background()
	require.NoError(t, follows.Follow(ctx, "u1", "u2"))
	require.NoError(t, follows.Follow(ctx, "u2", "u1"))
	require.NoError(t, follows.Follow(ctx, "u3", "u1"))

	require.NoError(t, repo.BlockAndUnfollow(ctx, "u1", "u2"))
	require.NoError(t, repo.BlockAndUnfollow(ctx, "u1", "u2"))

	ok, _ := follows.IsFollowing(ctx, "u1", "u2")
	assert.False(t, ok)
	ok, _ = follows.IsFollowing(ctx, "u2", "u1")
	assert.False(t, ok)
	ok, _ = follows.IsFollowing(ctx, "u3", "u1")
	assert.True(t, ok)
	ok, _ = repo.IsBlocking(ctx, "u1", "u2")
	assert.True(t, ok)
}

func TestBlockRepository_ListBlockedNewestFirst(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewBlockRepository(db)
	ctx := context.Background()
	for _, id := range []string{"u1", "u2", "u3"} {
		require.NoError(t, db.Create(&models.User{ID: id, Email: id + "@x.com", Username: id, DisplayName: "D " + id, Bio: "bio " + id}).Error)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.Block{BlockerID: "u1", BlockedID: "u2", CreatedAt: base}).Error)
	require.NoError(t, db.Create(&models.Block{BlockerID: "u1", BlockedID: "u3", CreatedAt: base.Add(time.Hour)}).Error)

	list, err := repo.ListBlocked(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "u3", list[0].Username)
	assert.Equal(t, "u2", list[1].Username)
	assert.Equal(t, "bio u2", list[1].Bio)

	list, err = repo.ListBlocked(ctx, "u2")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestBlockRepository_BlockAndUnfollowDeletesRequestsBothWays(t *testing.T) {
	db := testutil.NewTestDB(t)
	blocks := NewBlockRepository(db)
	follows := NewFollowRepository(db)
	ctx := context.Background()

	require.NoError(t, follows.Request(ctx, "u1", "u2"))
	require.NoError(t, follows.Request(ctx, "u2", "u1"))
	require.NoError(t, follows.Request(ctx, "u3", "u1"))

	require.NoError(t, blocks.BlockAndUnfollow(ctx, "u1", "u2"))

	for _, p := range [][2]string{{"u1", "u2"}, {"u2", "u1"}} {
		has, err := follows.HasRequest(ctx, p[0], p[1])
		require.NoError(t, err)
		assert.False(t, has)
	}
	has, err := follows.HasRequest(ctx, "u3", "u1")
	require.NoError(t, err)
	assert.True(t, has)
}
