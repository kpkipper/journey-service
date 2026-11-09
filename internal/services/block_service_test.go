package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockService(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	users := repository.NewUserRepository(f.db)
	blocks := repository.NewBlockRepository(f.db)
	bsvc := NewBlockService(users, blocks, f.follows)
	fsvc := NewFollowService(users, f.follows).WithBlocks(blocks)

	t.Run("block removes follows both ways, idempotent", func(t *testing.T) {
		require.NoError(t, f.follows.Follow(ctx, "alice", "bob"))
		require.NoError(t, f.follows.Follow(ctx, "bob", "alice"))
		require.NoError(t, bsvc.Block(ctx, "alice", "Bob"))
		require.NoError(t, bsvc.Block(ctx, "alice", "bob"))
		ok, _ := f.follows.IsFollowing(ctx, "alice", "bob")
		assert.False(t, ok)
		ok, _ = f.follows.IsFollowing(ctx, "bob", "alice")
		assert.False(t, ok)
		ok, _ = f.follows.IsFollowing(ctx, "fol", "alice") // third party untouched
		assert.True(t, ok)
	})

	t.Run("follow after block fails both sides, unblock restores", func(t *testing.T) {
		assert.ErrorIs(t, fsvc.Follow(ctx, "alice", "bob"), ErrUserNotFound)
		assert.ErrorIs(t, fsvc.Follow(ctx, "bob", "alice"), ErrUserNotFound)
		require.NoError(t, fsvc.Follow(ctx, "str", "bob"))

		require.NoError(t, bsvc.Unblock(ctx, "alice", "bob"))
		require.NoError(t, bsvc.Unblock(ctx, "alice", "bob"))
		require.NoError(t, fsvc.Follow(ctx, "bob", "alice"))
	})

	t.Run("self and unknown", func(t *testing.T) {
		var ve *ValidationError
		err := bsvc.Block(ctx, "alice", "alice")
		require.True(t, errors.As(err, &ve))
		assert.Equal(t, "cannot block yourself", ve.Message)
		assert.ErrorIs(t, bsvc.Block(ctx, "alice", "nobody"), ErrUserNotFound)
	})

	t.Run("list", func(t *testing.T) {
		require.NoError(t, bsvc.Block(ctx, "alice", "str"))
		l, err := bsvc.List(ctx, "alice")
		require.NoError(t, err)
		require.Len(t, l, 1)
		assert.Equal(t, "str", l[0].Username)
	})
}

func TestProfile_Blocks(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	blocks := repository.NewBlockRepository(f.db)
	f.svc.Blocks = blocks
	users := repository.NewUserRepository(f.db)
	bsvc := NewBlockService(users, blocks, f.follows)
	require.NoError(t, f.follows.Follow(ctx, "bob", "alice"))
	require.NoError(t, bsvc.Block(ctx, "alice", "bob"))
	// bob has a public trip and an ongoing one: the blocker must still see nothing.
	f.trip(t, pTrip{owner: "bob", visibility: "public", dates: [2]time.Time{privacyNow.AddDate(0, -3, 0), privacyNow.AddDate(0, -3, 5)}})
	f.trip(t, pTrip{owner: "bob", visibility: "public", dates: [2]time.Time{privacyNow.AddDate(0, 0, -1), privacyNow.AddDate(0, 0, 3)}})
	p0, err := f.svc.Get(ctx, "str", "bob") // sanity: a stranger does see them
	require.NoError(t, err)
	require.NotEmpty(t, p0.Upcoming) // the ongoing trip, redacted for a stranger
	require.NotEmpty(t, p0.Past)

	// blocked viewer: not found
	_, err = f.svc.Get(ctx, "bob", "alice")
	assert.ErrorIs(t, err, ErrUserNotFound)

	// blocker sees the blocked user's empty profile
	p, err := f.svc.Get(ctx, "alice", "bob")
	require.NoError(t, err)
	assert.True(t, p.IsBlocked)
	assert.False(t, p.IsFollowing)
	assert.Nil(t, p.Now)
	assert.Empty(t, p.Upcoming)
	assert.Empty(t, p.Calendar)
	assert.Empty(t, p.Past)
	assert.NotNil(t, p.Upcoming)
	assert.Equal(t, ProfileStats{}, p.Stats)
	assert.Equal(t, "bob", p.User.Username)

	// third party unaffected
	p, err = f.svc.Get(ctx, "str", "alice")
	require.NoError(t, err)
	assert.False(t, p.IsBlocked)
	assert.Equal(t, int64(1), p.Followers) // fol only; bob's follow was removed
}

func TestProfile_MutualBlockBlockedByThemWins(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	blocks := repository.NewBlockRepository(f.db)
	f.svc.Blocks = blocks
	require.NoError(t, blocks.Block(ctx, "alice", "bob"))
	require.NoError(t, blocks.Block(ctx, "bob", "alice"))
	_, err := f.svc.Get(ctx, "alice", "bob")
	assert.ErrorIs(t, err, ErrUserNotFound)
	_, err = f.svc.Get(ctx, "bob", "alice")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestUserService_PublicProfileFor(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	repo := repository.NewUserRepository(db)
	svc := NewUserService(repo)
	a := mkUser(t, repo, "a@x.com", "alice")
	b := mkUser(t, repo, "b@x.com", "bobby")
	c := mkUser(t, repo, "c@x.com", "carol")
	_ = c
	// no blocks configured: works for anyone
	p, err := svc.PublicProfileFor(ctx, b.ID, "alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", p.Username)

	blocks := repository.NewBlockRepository(db)
	svc.WithBlocks(blocks)
	require.NoError(t, blocks.Block(ctx, a.ID, b.ID))

	_, err = svc.PublicProfileFor(ctx, b.ID, "alice")
	assert.ErrorIs(t, err, ErrUserNotFound)
	_, err = svc.PublicProfileFor(ctx, a.ID, "bobby") // blocker still sees
	assert.NoError(t, err)
	_, err = svc.PublicProfileFor(ctx, c.ID, "alice")
	assert.NoError(t, err)
	_, err = svc.PublicProfileFor(ctx, "", "alice") // anonymous
	assert.NoError(t, err)
}
