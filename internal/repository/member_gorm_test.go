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

func TestMemberRepository_UniquePerJourney(t *testing.T) {
	repo := NewMemberRepository(testutil.NewTestDB(t))
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &models.JourneyMember{JourneyID: "j1", UserID: "u1", Role: models.RoleEditor, Status: models.StatusJoined}))
	assert.Error(t, repo.Create(ctx, &models.JourneyMember{JourneyID: "j1", UserID: "u1", Role: models.RoleTraveler, Status: models.StatusInvited}))
	assert.NoError(t, repo.Create(ctx, &models.JourneyMember{JourneyID: "j2", UserID: "u1", Role: models.RoleTraveler, Status: models.StatusInvited}))
}

func TestMemberRepository_ListByJourney(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := NewMemberRepository(db)
	ctx := context.Background()

	require.NoError(t, db.Create(&models.User{ID: "u1", Email: "a@x.com", Username: "alice", DisplayName: "Alice"}).Error)
	require.NoError(t, db.Create(&models.User{ID: "u2", Email: "b@x.com", Username: "bob"}).Error)

	// invited is created first, so ordering must come from status, not created_at alone
	require.NoError(t, repo.Create(ctx, &models.JourneyMember{JourneyID: "j1", UserID: "u2", Role: models.RoleTraveler, Status: models.StatusInvited}))
	require.NoError(t, repo.Create(ctx, &models.JourneyMember{JourneyID: "j1", UserID: "u1", Role: models.RoleEditor, Status: models.StatusJoined}))

	got, err := repo.ListByJourney(ctx, "j1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "alice", got[0].Username)
	assert.Equal(t, "Alice", got[0].DisplayName)
	assert.Equal(t, models.RoleEditor, got[0].Role)
	assert.Equal(t, models.StatusJoined, got[0].Status)
	assert.Equal(t, "bob", got[1].Username)
	assert.Equal(t, models.StatusInvited, got[1].Status)
}

func TestMemberRepository_CountActive(t *testing.T) {
	repo := NewMemberRepository(testutil.NewTestDB(t))
	ctx := context.Background()

	a := &models.JourneyMember{JourneyID: "j1", UserID: "u1", Role: models.RoleEditor, Status: models.StatusJoined}
	b := &models.JourneyMember{JourneyID: "j1", UserID: "u2", Role: models.RoleTraveler, Status: models.StatusInvited}
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))
	n, err := repo.CountActive(ctx, "j1")
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	require.NoError(t, repo.Delete(ctx, b.ID))
	n, err = repo.CountActive(ctx, "j1")
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
}

func TestMemberRepository_GetUpdateAndUserLists(t *testing.T) {
	repo := NewMemberRepository(testutil.NewTestDB(t))
	ctx := context.Background()

	m := &models.JourneyMember{JourneyID: "j1", UserID: "u1", Role: models.RoleTraveler, Status: models.StatusInvited}
	require.NoError(t, repo.Create(ctx, m))
	require.NotEmpty(t, m.ID)

	got, err := repo.Get(ctx, "j1", "u1")
	require.NoError(t, err)
	require.NotNil(t, got)
	byID, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	for _, f := range []func() (*models.JourneyMember, error){
		func() (*models.JourneyMember, error) { return repo.Get(ctx, "j1", "nope") },
		func() (*models.JourneyMember, error) { return repo.GetByID(ctx, "nope") },
	} {
		g, err := f()
		assert.NoError(t, err)
		assert.Nil(t, g)
	}

	invites, err := repo.ListInvitesForUser(ctx, "u1")
	require.NoError(t, err)
	assert.Len(t, invites, 1)
	ids, err := repo.ListJourneyIDsForUser(ctx, "u1")
	require.NoError(t, err)
	assert.Empty(t, ids)

	m.Status = models.StatusJoined
	require.NoError(t, repo.Update(ctx, m))
	invites, err = repo.ListInvitesForUser(ctx, "u1")
	require.NoError(t, err)
	assert.Empty(t, invites)
	ids, err = repo.ListJourneyIDsForUser(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, []string{"j1"}, ids)
}

func TestMemberRepository_InviteLinks(t *testing.T) {
	repo := NewMemberRepository(testutil.NewTestDB(t))
	ctx := context.Background()
	exp := time.Now().Add(7 * 24 * time.Hour)

	l1 := &models.InviteLink{JourneyID: "j1", TokenHash: "h1", Role: models.RoleTraveler, CreatedBy: "u1", ExpiresAt: exp}
	l2 := &models.InviteLink{JourneyID: "j1", TokenHash: "h2", Role: models.RoleEditor, CreatedBy: "u1", ExpiresAt: exp}
	require.NoError(t, repo.CreateInviteLink(ctx, l1))
	require.NoError(t, repo.CreateInviteLink(ctx, l2))
	require.NotEmpty(t, l1.ID)

	require.NoError(t, repo.RevokeInviteLink(ctx, l1.ID, time.Now()))
	list, err := repo.ListInviteLinks(ctx, "j1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, l2.ID, list[0].ID)

	byHash, err := repo.GetInviteLinkByHash(ctx, "h1")
	require.NoError(t, err)
	require.NotNil(t, byHash)
	assert.NotNil(t, byHash.RevokedAt)

	none, err := repo.GetInviteLinkByHash(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, none)

	g, err := repo.GetInviteLink(ctx, "j1", l2.ID)
	require.NoError(t, err)
	assert.NotNil(t, g)
	g, err = repo.GetInviteLink(ctx, "other-trip", l2.ID)
	assert.NoError(t, err)
	assert.Nil(t, g)
}
