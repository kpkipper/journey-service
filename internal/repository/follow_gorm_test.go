package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func seedUsers(t *testing.T, db *gorm.DB, names ...string) map[string]string {
	t.Helper()
	ids := map[string]string{}
	for _, n := range names {
		u := &models.User{Email: n + "@x.com", Username: n, DisplayName: n + " D", Bio: n + " bio", AvatarURL: "http://a/" + n}
		require.NoError(t, db.Create(u).Error)
		ids[n] = u.ID
	}
	return ids
}

func TestFollowRepository_RequestIdempotentKeepsCreatedAt(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewFollowRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Request(ctx, "u1", "u2"))
	old := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, db.Model(&models.FollowRequest{}).Where("requester_id = ?", "u1").Update("created_at", old).Error)
	require.NoError(t, repo.Request(ctx, "u1", "u2"))

	var fr models.FollowRequest
	require.NoError(t, db.First(&fr, "requester_id = ? AND target_id = ?", "u1", "u2").Error)
	assert.True(t, fr.CreatedAt.Equal(old))
	n, err := repo.CountRequests(ctx, "u2")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	ok, err := repo.HasRequest(ctx, "u1", "u2")
	require.NoError(t, err)
	assert.True(t, ok)
	require.NoError(t, repo.Request(ctx, "u1", "u3"))
	out, err := repo.CountOutgoingRequests(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), out)
}

func TestFollowRepository_CancelRequest(t *testing.T) {
	repo := NewFollowRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	require.NoError(t, repo.Request(ctx, "u1", "u2"))
	require.NoError(t, repo.CancelRequest(ctx, "u1", "u2"))
	require.NoError(t, repo.CancelRequest(ctx, "u1", "u2"))
	ok, err := repo.HasRequest(ctx, "u1", "u2")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFollowRepository_AcceptRequest(t *testing.T) {
	repo := NewFollowRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	require.NoError(t, repo.Request(ctx, "u1", "u2"))

	ok, err := repo.AcceptRequest(ctx, "u1", "u2")
	require.NoError(t, err)
	assert.True(t, ok)
	f, err := repo.IsFollowing(ctx, "u1", "u2")
	require.NoError(t, err)
	assert.True(t, f)
	has, _ := repo.HasRequest(ctx, "u1", "u2")
	assert.False(t, has)

	ok, err = repo.AcceptRequest(ctx, "u1", "u2")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFollowRepository_AcceptAllRequestsSkips(t *testing.T) {
	repo := NewFollowRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	for _, r := range []string{"u1", "u3", "u4"} {
		require.NoError(t, repo.Request(ctx, r, "u2"))
	}
	require.NoError(t, repo.Request(ctx, "u1", "u5"))

	require.NoError(t, repo.AcceptAllRequests(ctx, "u2", map[string]bool{"u3": true}))

	n, _ := repo.CountFollowers(ctx, "u2")
	assert.Equal(t, int64(2), n)
	f, _ := repo.IsFollowing(ctx, "u3", "u2")
	assert.False(t, f)
	c, _ := repo.CountRequests(ctx, "u2")
	assert.Equal(t, int64(0), c)
	c, _ = repo.CountRequests(ctx, "u5")
	assert.Equal(t, int64(1), c)

	// empty target is fine
	require.NoError(t, repo.AcceptAllRequests(ctx, "nobody", nil))
}

func TestFollowRepository_ListRequestsNewestFirst(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewFollowRepository(db)
	ctx := context.Background()
	ids := seedUsers(t, db, "alice", "bob", "carol", "target")

	empty, err := repo.ListRequests(ctx, ids["target"], 10)
	require.NoError(t, err)
	require.NotNil(t, empty)
	assert.Len(t, empty, 0)

	base := time.Now().UTC().Truncate(time.Second)
	for i, n := range []string{"alice", "bob", "carol"} {
		require.NoError(t, db.Create(&models.FollowRequest{RequesterID: ids[n], TargetID: ids["target"], CreatedAt: base.Add(time.Duration(i) * time.Minute)}).Error)
	}

	got, err := repo.ListRequests(ctx, ids["target"], 10)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "carol", got[0].User.Username)
	assert.Equal(t, "bob", got[1].User.Username)
	assert.Equal(t, "alice", got[2].User.Username)
	assert.Equal(t, ids["carol"], got[0].UserID)
	assert.Equal(t, ids["alice"], got[2].UserID)
	assert.Equal(t, "carol D", got[0].User.DisplayName)
	assert.Equal(t, "carol bio", got[0].User.Bio)
	assert.Equal(t, "http://a/carol", got[0].User.AvatarURL)
	assert.True(t, got[0].CreatedAt.Equal(base.Add(2*time.Minute)))

	limited, err := repo.ListRequests(ctx, ids["target"], 2)
	require.NoError(t, err)
	assert.Len(t, limited, 2)
}

func TestFollowRepository_ListFollowers(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewFollowRepository(db)
	ctx := context.Background()
	ids := seedUsers(t, db, "alice", "bob", "target")

	empty, err := repo.ListFollowers(ctx, ids["target"], 10)
	require.NoError(t, err)
	require.NotNil(t, empty)

	base := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, db.Create(&models.Follow{FollowerID: ids["alice"], FolloweeID: ids["target"], CreatedAt: base}).Error)
	require.NoError(t, db.Create(&models.Follow{FollowerID: ids["bob"], FolloweeID: ids["target"], CreatedAt: base.Add(time.Minute)}).Error)

	got, err := repo.ListFollowers(ctx, ids["target"], 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "bob", got[0].User.Username)
	assert.Equal(t, "alice", got[1].User.Username)
	assert.Equal(t, "alice bio", got[1].User.Bio)
	assert.Equal(t, ids["bob"], got[0].UserID)
	assert.Equal(t, ids["alice"], got[1].UserID)
	assert.True(t, got[0].CreatedAt.Equal(base.Add(time.Minute)))
}

func TestFollowRepository_AcceptSkipsBlockedPairs(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewFollowRepository(db)
	blocks := NewBlockRepository(db)
	ctx := context.Background()
	// request rows that survived a racing block (block rows only, no cleanup)
	require.NoError(t, repo.Request(ctx, "u1", "u2"))
	require.NoError(t, repo.Request(ctx, "u3", "u2"))
	require.NoError(t, repo.Request(ctx, "u4", "u2"))
	require.NoError(t, blocks.Block(ctx, "u2", "u1")) // target blocked requester
	require.NoError(t, blocks.Block(ctx, "u3", "u2")) // requester blocked target

	ok, err := repo.AcceptRequest(ctx, "u1", "u2")
	require.NoError(t, err)
	assert.False(t, ok)
	f, _ := repo.IsFollowing(ctx, "u1", "u2")
	assert.False(t, f)

	require.NoError(t, repo.AcceptAllRequests(ctx, "u2", nil))
	f, _ = repo.IsFollowing(ctx, "u3", "u2")
	assert.False(t, f)
	f, _ = repo.IsFollowing(ctx, "u4", "u2")
	assert.True(t, f)
	n, _ := repo.CountFollowers(ctx, "u2")
	assert.Equal(t, int64(1), n)
}

func TestFollowRepository_CountRequestsExcludesBlocked(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewFollowRepository(db)
	blocks := NewBlockRepository(db)
	ctx := context.Background()
	for _, r := range []string{"u1", "u3", "u4"} {
		require.NoError(t, repo.Request(ctx, r, "u2"))
	}
	require.NoError(t, blocks.Block(ctx, "u2", "u1"))
	require.NoError(t, blocks.Block(ctx, "u3", "u2"))
	n, err := repo.CountRequests(ctx, "u2")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}
