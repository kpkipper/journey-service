package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setApproval(t *testing.T, f *profileFixture, id string, on bool) {
	require.NoError(t, repository.NewUserRepository(f.db).SetFollowApproval(context.Background(), id, on))
}

func entryNames(es []models.FollowEntry) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.User.Username)
	}
	return out
}

func TestFollow_Requests(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	blocks := repository.NewBlockRepository(f.db)
	users := repository.NewUserRepository(f.db)
	svc := NewFollowService(users, f.follows).WithBlocks(blocks)

	t.Run("approval on creates a request, idempotent", func(t *testing.T) {
		st, err := svc.Follow(ctx, "bob", "alice")
		require.NoError(t, err)
		assert.Equal(t, FollowState{Requested: true, Followers: 1}, st)
		st, err = svc.Follow(ctx, "bob", "alice")
		require.NoError(t, err)
		assert.True(t, st.Requested)
		ok, _ := f.follows.IsFollowing(ctx, "bob", "alice")
		assert.False(t, ok)
		n, _ := svc.RequestCount(ctx, "alice")
		assert.Equal(t, int64(1), n)
	})

	t.Run("already following creates no request", func(t *testing.T) {
		st, err := svc.Follow(ctx, "fol", "alice")
		require.NoError(t, err)
		assert.Equal(t, FollowState{IsFollowing: true, Followers: 1}, st)
		has, _ := f.follows.HasRequest(ctx, "fol", "alice")
		assert.False(t, has)
	})

	t.Run("approval off follows instantly", func(t *testing.T) {
		setApproval(t, f, "kmem", false)
		st, err := svc.Follow(ctx, "bob", "kmem")
		require.NoError(t, err)
		assert.Equal(t, FollowState{IsFollowing: true, Followers: 1}, st)
	})

	t.Run("unfollow cancels request and follow", func(t *testing.T) {
		st, err := svc.Unfollow(ctx, "bob", "alice")
		require.NoError(t, err)
		assert.Equal(t, FollowState{Followers: 1}, st)
		has, _ := f.follows.HasRequest(ctx, "bob", "alice")
		assert.False(t, has)
		st, err = svc.Unfollow(ctx, "fol", "alice")
		require.NoError(t, err)
		assert.Equal(t, int64(0), st.Followers)
		require.NoError(t, f.follows.Follow(ctx, "fol", "alice"))
	})

	t.Run("accept, decline, remove follower", func(t *testing.T) {
		_, _ = svc.Follow(ctx, "bob", "alice")
		_, _ = svc.Follow(ctx, "str", "alice")
		reqs, err := svc.Requests(ctx, "alice")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"bob", "str"}, entryNames(reqs))

		require.NoError(t, svc.Accept(ctx, "alice", "Bob"))
		ok, _ := f.follows.IsFollowing(ctx, "bob", "alice")
		assert.True(t, ok)
		assert.ErrorIs(t, svc.Accept(ctx, "alice", "bob"), ErrUserNotFound, "no request any more")
		assert.ErrorIs(t, svc.Accept(ctx, "alice", "nobody"), ErrUserNotFound)

		require.NoError(t, svc.Decline(ctx, "alice", "str"))
		require.NoError(t, svc.Decline(ctx, "alice", "str"))
		assert.ErrorIs(t, svc.Decline(ctx, "alice", "nobody"), ErrUserNotFound)
		reqs, _ = svc.Requests(ctx, "alice")
		assert.Empty(t, reqs)
		assert.NotNil(t, reqs)

		fl, err := svc.Followers(ctx, "alice")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"bob", "fol"}, entryNames(fl))

		require.NoError(t, svc.RemoveFollower(ctx, "alice", "bob"))
		require.NoError(t, svc.RemoveFollower(ctx, "alice", "bob"))
		assert.ErrorIs(t, svc.RemoveFollower(ctx, "alice", "nobody"), ErrUserNotFound)
		ok, _ = f.follows.IsFollowing(ctx, "bob", "alice")
		assert.False(t, ok)
	})

	t.Run("blocks hide requests and followers, accept 404", func(t *testing.T) {
		_, err := svc.Follow(ctx, "str", "alice")
		require.NoError(t, err)
		// block rows only (no cleanup), as if raced
		require.NoError(t, blocks.Block(ctx, "alice", "str"))
		reqs, _ := svc.Requests(ctx, "alice")
		assert.Empty(t, reqs)
		assert.ErrorIs(t, svc.Accept(ctx, "alice", "str"), ErrUserNotFound)

		require.NoError(t, blocks.Block(ctx, "alice", "fol"))
		fl, _ := svc.Followers(ctx, "alice")
		assert.NotContains(t, entryNames(fl), "fol")
		require.NoError(t, blocks.Unblock(ctx, "alice", "fol"))
		require.NoError(t, blocks.Unblock(ctx, "alice", "str"))
	})

	t.Run("outgoing cap of 200", func(t *testing.T) {
		for i := 0; i < 199; i++ { // kmem has 0 pending now; add 199 ghost targets
			require.NoError(t, f.follows.Request(ctx, "kmem", fmt.Sprintf("g%d", i)))
		}
		_, err := svc.Follow(ctx, "kmem", "alice") // 200th
		require.NoError(t, err)
		_, err = svc.Follow(ctx, "kmem", "bob") // 201st
		assert.ErrorIs(t, err, ErrTooManyRequests)
		st, err := svc.Follow(ctx, "kmem", "alice") // existing request still ok
		require.NoError(t, err)
		assert.True(t, st.Requested)
	})
}

func TestUpdateProfile_FollowApproval(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	repo := repository.NewUserRepository(f.db)
	blocks := repository.NewBlockRepository(f.db)
	svc := NewUserService(repo).WithBlocks(blocks).WithFollows(f.follows)

	off, on, dn := false, true, "A"
	u, err := svc.UpdateProfile(ctx, "alice", models.ProfileUpdate{DisplayName: &dn})
	require.NoError(t, err)
	assert.True(t, u.FollowApproval, "unchanged when omitted")

	require.NoError(t, f.follows.Request(ctx, "bob", "alice"))
	require.NoError(t, f.follows.Request(ctx, "str", "alice"))
	require.NoError(t, blocks.Block(ctx, "alice", "str"))
	require.NoError(t, f.follows.Request(ctx, "str", "alice")) // re-added after block

	u, err = svc.UpdateProfile(ctx, "alice", models.ProfileUpdate{FollowApproval: &off})
	require.NoError(t, err)
	assert.False(t, u.FollowApproval)
	got, _ := repo.GetByID(ctx, "alice")
	assert.False(t, got.FollowApproval, "persisted")
	ok, _ := f.follows.IsFollowing(ctx, "bob", "alice")
	assert.True(t, ok, "pending accepted")
	ok, _ = f.follows.IsFollowing(ctx, "str", "alice")
	assert.False(t, ok, "blocked requester skipped")
	n, _ := f.follows.CountRequests(ctx, "alice")
	assert.Equal(t, int64(0), n)

	u, err = svc.UpdateProfile(ctx, "alice", models.ProfileUpdate{FollowApproval: &on})
	require.NoError(t, err)
	assert.True(t, u.FollowApproval)
	got, _ = repo.GetByID(ctx, "alice")
	assert.True(t, got.FollowApproval)
}

func TestProfile_FollowFields(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	require.NoError(t, f.follows.Request(ctx, "bob", "alice"))
	p, err := f.svc.Get(ctx, "bob", "alice")
	require.NoError(t, err)
	assert.True(t, p.FollowRequested)
	assert.True(t, p.FollowApproval)
	assert.False(t, p.IsFollowing)
	p, _ = f.svc.Get(ctx, "str", "alice")
	assert.False(t, p.FollowRequested)
}

func TestUpdateProfile_FollowApprovalOffClearsStaleRequests(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	repo := repository.NewUserRepository(f.db)
	svc := NewUserService(repo).WithBlocks(repository.NewBlockRepository(f.db)).WithFollows(f.follows)
	off := false
	// approval is already off (flag false), but a request slipped in
	require.NoError(t, repo.SetFollowApproval(ctx, "alice", false))
	require.NoError(t, f.follows.Request(ctx, "bob", "alice"))
	_, err := svc.UpdateProfile(ctx, "alice", models.ProfileUpdate{FollowApproval: &off})
	require.NoError(t, err)
	ok, _ := f.follows.IsFollowing(ctx, "bob", "alice")
	assert.True(t, ok)
	n, _ := f.follows.CountRequests(ctx, "alice")
	assert.Zero(t, n)
}

func TestFollow_InstantFollowClearsStaleRequest(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	svc := NewFollowService(repository.NewUserRepository(f.db), f.follows)
	require.NoError(t, repository.NewUserRepository(f.db).SetFollowApproval(ctx, "alice", false))
	require.NoError(t, f.follows.Request(ctx, "bob", "alice"))
	st, err := svc.Follow(ctx, "bob", "alice")
	require.NoError(t, err)
	assert.True(t, st.IsFollowing)
	has, _ := f.follows.HasRequest(ctx, "bob", "alice")
	assert.False(t, has)
}

func TestFollow_BlockRelatedUsernamesAre404AfterCleanup(t *testing.T) {
	ctx := context.Background()
	f := newProfileFixture(t)
	blocks := repository.NewBlockRepository(f.db)
	svc := NewFollowService(repository.NewUserRepository(f.db), f.follows).WithBlocks(blocks)
	// rows that survived a racing block (block rows only, no cleanup)
	require.NoError(t, f.follows.Request(ctx, "str", "alice"))
	require.NoError(t, f.follows.Follow(ctx, "bob", "alice"))
	require.NoError(t, f.follows.Follow(ctx, "alice", "kmem"))
	require.NoError(t, f.follows.Request(ctx, "alice", "kmem"))
	require.NoError(t, blocks.Block(ctx, "alice", "str"))
	require.NoError(t, blocks.Block(ctx, "bob", "alice"))
	require.NoError(t, blocks.Block(ctx, "kmem", "alice"))

	assert.ErrorIs(t, svc.Decline(ctx, "alice", "str"), ErrUserNotFound)
	has, _ := f.follows.HasRequest(ctx, "str", "alice")
	assert.False(t, has, "declined anyway")

	assert.ErrorIs(t, svc.RemoveFollower(ctx, "alice", "bob"), ErrUserNotFound)
	ok, _ := f.follows.IsFollowing(ctx, "bob", "alice")
	assert.False(t, ok, "removed anyway")

	_, err := svc.Unfollow(ctx, "alice", "kmem")
	assert.ErrorIs(t, err, ErrUserNotFound)
	ok, _ = f.follows.IsFollowing(ctx, "alice", "kmem")
	assert.False(t, ok, "unfollowed anyway")
	has, _ = f.follows.HasRequest(ctx, "alice", "kmem")
	assert.False(t, has, "request cancelled anyway")
}
