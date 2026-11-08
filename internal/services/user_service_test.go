package services

import (
	"context"
	"strings"
	"testing"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUserSvc(t *testing.T) (*UserService, repository.UserRepository) {
	t.Helper()
	repo := repository.NewUserRepository(testutil.NewTestDB(t))
	return NewUserService(repo), repo
}

func mkUser(t *testing.T, repo repository.UserRepository, email, username string) *models.User {
	t.Helper()
	u := &models.User{Email: email, Username: username, DisplayName: "Name " + username}
	require.NoError(t, repo.Create(context.Background(), u))
	return u
}

func strp(s string) *string { return &s }

func TestGenerateUsername(t *testing.T) {
	ctx := context.Background()
	svc, repo := newUserSvc(t)

	got, err := svc.GenerateUsername(ctx, "Kip.Per+trip@gmail.com")
	require.NoError(t, err)
	assert.Equal(t, "kippertrip", got)

	got, err = svc.GenerateUsername(ctx, "a@x.com")
	require.NoError(t, err)
	assert.Equal(t, "traveler", got)

	mkUser(t, repo, "x@x.com", "kippertrip")
	got, err = svc.GenerateUsername(ctx, "Kip.Per+trip@gmail.com")
	require.NoError(t, err)
	assert.Regexp(t, `^kippertrip_[a-z0-9]{4}$`, got)

	got, err = svc.GenerateUsername(ctx, "abcdefghijklmnopqrstuvwxyz@x.com")
	require.NoError(t, err)
	assert.Equal(t, "abcdefghijklmno", got)
}

func TestUpdateProfile_Validation(t *testing.T) {
	ctx := context.Background()
	svc, repo := newUserSvc(t)
	u := mkUser(t, repo, "a@x.com", "alice")

	var ve *ValidationError
	_, err := svc.UpdateProfile(ctx, u.ID, models.ProfileUpdate{Username: strp("ab")})
	require.ErrorAs(t, err, &ve)
	assert.NotEmpty(t, ve.Error())

	got, err := svc.UpdateProfile(ctx, u.ID, models.ProfileUpdate{Username: strp("Kip_Per")})
	require.NoError(t, err)
	assert.Equal(t, "kip_per", got.Username)

	_, err = svc.UpdateProfile(ctx, u.ID, models.ProfileUpdate{Username: strp("kip per")})
	require.ErrorAs(t, err, &ve)

	_, err = svc.UpdateProfile(ctx, u.ID, models.ProfileUpdate{DisplayName: strp(strings.Repeat("ก", 51))})
	require.ErrorAs(t, err, &ve)
	_, err = svc.UpdateProfile(ctx, u.ID, models.ProfileUpdate{DisplayName: strp(strings.Repeat("ก", 50))})
	require.NoError(t, err)

	_, err = svc.UpdateProfile(ctx, u.ID, models.ProfileUpdate{Bio: strp(strings.Repeat("ก", 160))})
	require.NoError(t, err)
	_, err = svc.UpdateProfile(ctx, u.ID, models.ProfileUpdate{Bio: strp(strings.Repeat("ก", 161))})
	require.ErrorAs(t, err, &ve)
}

func TestUpdateProfile_UsernameTaken(t *testing.T) {
	ctx := context.Background()
	svc, repo := newUserSvc(t)
	a := mkUser(t, repo, "a@x.com", "alice")
	b := mkUser(t, repo, "b@x.com", "bob")

	_, err := svc.UpdateProfile(ctx, b.ID, models.ProfileUpdate{Username: strp("Alice")})
	assert.ErrorIs(t, err, ErrUsernameTaken)

	got, err := svc.UpdateProfile(ctx, a.ID, models.ProfileUpdate{Username: strp("alice")})
	require.NoError(t, err)
	assert.Equal(t, "alice", got.Username)
}

func TestUpdateProfile_PartialUpdate(t *testing.T) {
	ctx := context.Background()
	svc, repo := newUserSvc(t)
	u := mkUser(t, repo, "a@x.com", "alice")

	got, err := svc.UpdateProfile(ctx, u.ID, models.ProfileUpdate{Bio: strp("hello")})
	require.NoError(t, err)
	assert.Equal(t, "hello", got.Bio)
	assert.Equal(t, "Name alice", got.DisplayName)
	assert.Equal(t, "alice", got.Username)

	_, err = svc.UpdateProfile(ctx, "nope", models.ProfileUpdate{Bio: strp("x")})
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestGetPublicProfile(t *testing.T) {
	ctx := context.Background()
	svc, repo := newUserSvc(t)
	mkUser(t, repo, "a@x.com", "alice")

	p, err := svc.GetPublicProfile(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", p.Username)
	assert.Equal(t, "Name alice", p.DisplayName)

	_, err = svc.GetPublicProfile(ctx, "ghost")
	assert.ErrorIs(t, err, ErrUserNotFound)
}
