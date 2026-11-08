package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type googleEnv struct {
	svc    *AuthService
	google *testutil.FakeGoogle
	users  repository.UserRepository
	db     *gorm.DB
}

func newGoogleEnv(t *testing.T) *googleEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	users := repository.NewUserRepository(db)
	g := &testutil.FakeGoogle{Identities: map[string]*auth.GoogleIdentity{}}
	svc := NewAuthService(AuthDeps{
		Users:   users,
		Auth:    repository.NewAuthRepository(db),
		UserSvc: NewUserService(users),
		Mailer:  &testutil.CaptureMailer{},
		Google:  g,
		WebURL:  "http://web.test",
		Now:     func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) },
	})
	return &googleEnv{svc: svc, google: g, users: users, db: db}
}

func (e *googleEnv) userCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, e.db.Model(&models.User{}).Count(&n).Error)
	return n
}

func kipIdentity() *auth.GoogleIdentity {
	return &auth.GoogleIdentity{Subject: "sub-1", Email: "kip@x.com", EmailVerified: true, Name: "Kip Per", Picture: "http://pic/1.png"}
}

func TestGoogleLogin_NewUser(t *testing.T) {
	e := newGoogleEnv(t)
	e.google.Identities["tok"] = kipIdentity()

	sess, u, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	require.NoError(t, err)
	assert.NotEmpty(t, sess)
	assert.Equal(t, "Kip Per", u.DisplayName)
	assert.Equal(t, "http://pic/1.png", u.AvatarURL)
	assert.Equal(t, "kip@x.com", u.Email)
	require.NotNil(t, u.GoogleSub)
	assert.Equal(t, "sub-1", *u.GoogleSub)
	authed, err := e.svc.Authenticate(context.Background(), sess)
	require.NoError(t, err)
	assert.Equal(t, u.ID, authed.ID)
}

func TestGoogleLogin_TruncatesLongName(t *testing.T) {
	e := newGoogleEnv(t)
	id := kipIdentity()
	id.Name = strings.Repeat("ก", 80)
	e.google.Identities["tok"] = id

	_, u, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("ก", 50), u.DisplayName)
}

func TestGoogleLogin_ReturningUser(t *testing.T) {
	e := newGoogleEnv(t)
	e.google.Identities["tok"] = kipIdentity()
	_, first, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	require.NoError(t, err)
	_, second, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.EqualValues(t, 1, e.userCount(t))
}

func TestGoogleLogin_LinksExistingEmailUser(t *testing.T) {
	e := newGoogleEnv(t)
	existing := &models.User{Email: "kip@x.com", Username: "kip"}
	require.NoError(t, e.users.Create(context.Background(), existing))
	id := kipIdentity()
	id.Email = "  Kip@X.com "
	e.google.Identities["tok"] = id

	_, u, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	require.NoError(t, err)
	assert.Equal(t, existing.ID, u.ID)
	require.NotNil(t, u.GoogleSub)
	assert.Equal(t, "sub-1", *u.GoogleSub)
	got, err := e.users.GetByGoogleSub(context.Background(), "sub-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, existing.ID, got.ID)
	assert.EqualValues(t, 1, e.userCount(t))
}

func TestGoogleLogin_KeepsEditedProfile(t *testing.T) {
	e := newGoogleEnv(t)
	e.google.Identities["tok"] = kipIdentity()
	_, u, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	require.NoError(t, err)

	u.DisplayName = "Kipper"
	u.AvatarURL = "http://mine/me.png"
	require.NoError(t, e.users.Update(context.Background(), u))

	_, again, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	require.NoError(t, err)
	assert.Equal(t, "Kipper", again.DisplayName)
	assert.Equal(t, "http://mine/me.png", again.AvatarURL)
}

func TestGoogleLogin_UnverifiedEmail(t *testing.T) {
	e := newGoogleEnv(t)
	id := kipIdentity()
	id.EmailVerified = false
	e.google.Identities["tok"] = id

	_, _, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	assert.ErrorIs(t, err, ErrInvalidToken)
	assert.EqualValues(t, 0, e.userCount(t))
}

func TestGoogleLogin_BadToken(t *testing.T) {
	e := newGoogleEnv(t)
	_, _, err := e.svc.LoginWithGoogle(context.Background(), "unknown")
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestGoogleLogin_EmailOwnedByOtherGoogleAccount(t *testing.T) {
	e := newGoogleEnv(t)
	other := "other-sub"
	require.NoError(t, e.users.Create(context.Background(), &models.User{Email: "kip@x.com", Username: "kip", GoogleSub: &other}))
	e.google.Identities["tok"] = kipIdentity()

	_, _, err := e.svc.LoginWithGoogle(context.Background(), "tok")
	assert.ErrorIs(t, err, ErrInvalidToken)
	got, err := e.users.GetByEmail(context.Background(), "kip@x.com")
	require.NoError(t, err)
	assert.Equal(t, "other-sub", *got.GoogleSub)
}
