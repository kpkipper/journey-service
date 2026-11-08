package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

type authEnv struct {
	svc   *AuthService
	mail  *testutil.CaptureMailer
	clock *fakeClock
	db    *gorm.DB
}

func newAuthEnv(t *testing.T) *authEnv {
	t.Helper()
	db := testutil.NewTestDB(t)
	users := repository.NewUserRepository(db)
	mail := &testutil.CaptureMailer{}
	clock := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	svc := NewAuthService(AuthDeps{
		Users:   users,
		Auth:    repository.NewAuthRepository(db),
		UserSvc: NewUserService(users),
		Mailer:  mail,
		WebURL:  "http://web.test",
		Now:     clock.now,
	})
	return &authEnv{svc: svc, mail: mail, clock: clock, db: db}
}

const verifyPrefix = "http://web.test/auth/verify?token="

func (e *authEnv) lastToken(t *testing.T) string {
	t.Helper()
	require.NotEmpty(t, e.mail.Sent)
	body := e.mail.Sent[len(e.mail.Sent)-1].Body
	i := strings.Index(body, verifyPrefix)
	require.GreaterOrEqual(t, i, 0)
	rest := body[i+len(verifyPrefix):]
	require.GreaterOrEqual(t, len(rest), 64)
	return rest[:64]
}

func (e *authEnv) login(t *testing.T, email string) (string, *models.User) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, e.svc.StartEmailLogin(ctx, email))
	sess, u, err := e.svc.VerifyEmailLogin(ctx, e.lastToken(t))
	require.NoError(t, err)
	return sess, u
}

func TestStartEmailLogin_SendsLink(t *testing.T) {
	e := newAuthEnv(t)
	require.NoError(t, e.svc.StartEmailLogin(context.Background(), "kip@gmail.com"))
	require.Len(t, e.mail.Sent, 1)
	assert.Equal(t, "kip@gmail.com", e.mail.Sent[0].To)
	assert.Equal(t, "เข้าสู่ระบบ Journey", e.mail.Sent[0].Subject)
	assert.Contains(t, e.mail.Sent[0].Body, verifyPrefix)
}

func TestEmailLogin_NewUser(t *testing.T) {
	e := newAuthEnv(t)
	sess, u := e.login(t, "kip@gmail.com")
	assert.Len(t, sess, 64)
	assert.Equal(t, "kip", u.Username)
	assert.Equal(t, "kip@gmail.com", u.Email)
}

func TestEmailLogin_NormalizesEmail(t *testing.T) {
	e := newAuthEnv(t)
	_, u1 := e.login(t, "  Kip@Gmail.com ")
	_, u2 := e.login(t, "kip@gmail.com")
	assert.Equal(t, u1.ID, u2.ID)
	assert.Equal(t, "kip@gmail.com", u2.Email)
	assert.Equal(t, "kip@gmail.com", e.mail.Sent[0].To)
}

func TestVerifyEmailLogin_ReusedToken(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	require.NoError(t, e.svc.StartEmailLogin(ctx, "kip@gmail.com"))
	tok := e.lastToken(t)
	_, _, err := e.svc.VerifyEmailLogin(ctx, tok)
	require.NoError(t, err)
	_, _, err = e.svc.VerifyEmailLogin(ctx, tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestVerifyEmailLogin_Unknown(t *testing.T) {
	e := newAuthEnv(t)
	_, _, err := e.svc.VerifyEmailLogin(context.Background(), "nope")
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestVerifyEmailLogin_Expired(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	require.NoError(t, e.svc.StartEmailLogin(ctx, "kip@gmail.com"))
	tok := e.lastToken(t)
	e.clock.advance(16 * time.Minute)
	_, _, err := e.svc.VerifyEmailLogin(ctx, tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
	var n int64
	require.NoError(t, e.db.Model(&models.Session{}).Count(&n).Error)
	assert.Zero(t, n)
}

func TestStartEmailLogin_RateLimit(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	for i := 0; i < MaxLoginLinksPerHour; i++ {
		require.NoError(t, e.svc.StartEmailLogin(ctx, "kip@gmail.com"))
	}
	err := e.svc.StartEmailLogin(ctx, "KIP@gmail.com")
	assert.ErrorIs(t, err, ErrTooManyRequests)
	assert.Len(t, e.mail.Sent, MaxLoginLinksPerHour)
	var n int64
	require.NoError(t, e.db.Model(&models.LoginToken{}).Count(&n).Error)
	assert.EqualValues(t, MaxLoginLinksPerHour, n)

	e.clock.advance(61 * time.Minute)
	assert.NoError(t, e.svc.StartEmailLogin(ctx, "kip@gmail.com"))
}

func TestStartEmailLogin_InvalidEmail(t *testing.T) {
	e := newAuthEnv(t)
	err := e.svc.StartEmailLogin(context.Background(), "not-an-email")
	var ve *ValidationError
	assert.True(t, errors.As(err, &ve))
	assert.Empty(t, e.mail.Sent)
}

func TestAuthenticate(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	sess, u := e.login(t, "kip@gmail.com")

	got, err := e.svc.Authenticate(ctx, sess)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)

	_, err = e.svc.Authenticate(ctx, "garbage")
	assert.ErrorIs(t, err, ErrUnauthorized)
	_, err = e.svc.Authenticate(ctx, "")
	assert.ErrorIs(t, err, ErrUnauthorized)

	e.clock.advance(31 * 24 * time.Hour)
	_, err = e.svc.Authenticate(ctx, sess)
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestLogout(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	sess, _ := e.login(t, "kip@gmail.com")
	require.NoError(t, e.svc.Logout(ctx, sess))
	_, err := e.svc.Authenticate(ctx, sess)
	assert.ErrorIs(t, err, ErrUnauthorized)
	assert.NoError(t, e.svc.Logout(ctx, "unknown"))
}
