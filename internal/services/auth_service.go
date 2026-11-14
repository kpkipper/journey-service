package services

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/mailer"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/pkg/logger"
)

const (
	SessionTTL           = 30 * 24 * time.Hour
	LoginTokenTTL        = 15 * time.Minute
	MaxLoginLinksPerHour = 5
	// MaxLoginLinksPerHourTotal caps login emails to everyone, so a flood of requests for
	// many different addresses can't get the SMTP account flagged for spam.
	MaxLoginLinksPerHourTotal = 200
	// LoginTokenRetention keeps used/expired login tokens long enough for the hourly limits.
	LoginTokenRetention = 24 * time.Hour
)

var (
	ErrInvalidToken    = errors.New("invalid or expired token")
	ErrTooManyRequests = errors.New("too many requests")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrEmailLoginOff   = errors.New("email login is off")
)

type AuthDeps struct {
	Users   repository.UserRepository
	Auth    repository.AuthRepository
	UserSvc *UserService
	Mailer  mailer.Mailer // nil = email login is off
	Google  auth.GoogleVerifier
	// GoogleEnabled says Google login is configured; it only feeds Methods.
	GoogleEnabled bool
	WebURL        string
	Now           func() time.Time // nil = time.Now
}

type AuthService struct {
	AuthDeps
}

func NewAuthService(d AuthDeps) *AuthService {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &AuthService{AuthDeps: d}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Methods reports which login methods the web app should offer.
func (s *AuthService) Methods() (email, google bool) {
	return s.Mailer != nil, s.GoogleEnabled
}

func (s *AuthService) StartEmailLogin(ctx context.Context, email string) error {
	if s.Mailer == nil {
		return ErrEmailLoginOff
	}
	email = normalizeEmail(email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return &ValidationError{Message: "invalid email address"}
	}

	now := s.Now()
	n, err := s.Auth.CountLoginTokensSince(ctx, email, now.Add(-time.Hour))
	if err != nil {
		return err
	}
	if n >= MaxLoginLinksPerHour {
		return ErrTooManyRequests
	}
	total, err := s.Auth.CountAllLoginTokensSince(ctx, now.Add(-time.Hour))
	if err != nil {
		return err
	}
	if total >= MaxLoginLinksPerHourTotal {
		logger.Get().Warn().Int64("sent_last_hour", total).Msg("login email total limit reached")
		return ErrTooManyRequests
	}

	token, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := s.Auth.CreateLoginToken(ctx, &models.LoginToken{
		Email:     email,
		TokenHash: auth.HashToken(token),
		ExpiresAt: now.Add(LoginTokenTTL),
		CreatedAt: now,
	}); err != nil {
		return err
	}

	body := "เข้าสู่ระบบ Journey ด้วยลิงก์นี้ (ใช้ได้ครั้งเดียว ภายใน 15 นาที):\n\n" +
		s.WebURL + "/auth/verify?token=" + token + "\n\n" +
		"ถ้าคุณไม่ได้ขอเข้าสู่ระบบ ไม่ต้องทำอะไร\n"
	return s.Mailer.Send(ctx, email, "เข้าสู่ระบบ Journey", body)
}

func (s *AuthService) VerifyEmailLogin(ctx context.Context, token string) (string, *models.User, error) {
	if s.Mailer == nil {
		return "", nil, ErrEmailLoginOff
	}
	if token == "" {
		return "", nil, ErrInvalidToken
	}
	lt, err := s.Auth.GetLoginTokenByHash(ctx, auth.HashToken(token))
	if err != nil {
		return "", nil, err
	}
	now := s.Now()
	if lt == nil || lt.UsedAt != nil || !now.Before(lt.ExpiresAt) {
		return "", nil, ErrInvalidToken
	}
	ok, err := s.Auth.MarkLoginTokenUsed(ctx, lt.ID, now)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, ErrInvalidToken
	}

	user, err := s.Users.GetByEmail(ctx, lt.Email)
	if err != nil {
		return "", nil, err
	}
	if user == nil {
		username, err := s.UserSvc.GenerateUsername(ctx, lt.Email)
		if err != nil {
			return "", nil, err
		}
		user = &models.User{Email: lt.Email, Username: username}
		if err := s.Users.Create(ctx, user); err != nil {
			return "", nil, err
		}
	}

	sessionToken, err := s.createSession(ctx, user.ID, now)
	if err != nil {
		return "", nil, err
	}
	return sessionToken, user, nil
}

func (s *AuthService) createSession(ctx context.Context, userID string, now time.Time) (string, error) {
	token, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	if err := s.Auth.CreateSession(ctx, &models.Session{
		UserID:    userID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: now.Add(SessionTTL),
		CreatedAt: now,
	}); err != nil {
		return "", err
	}
	return token, nil
}

const maxDisplayNameRunes = 50

// LoginWithGoogle signs a user in with a Google ID token. A verified Google email
// is linked to an existing account with the same email (unless that account is
// already bound to a different Google identity); otherwise a new user is created.
// Existing users' profile fields are never overwritten.
func (s *AuthService) LoginWithGoogle(ctx context.Context, idToken string) (string, *models.User, error) {
	id, err := s.Google.Verify(ctx, idToken)
	if err != nil {
		logger.Get().Warn().Err(err).Msg("google id token verification failed")
		return "", nil, ErrInvalidToken
	}
	if id == nil || !id.EmailVerified || id.Subject == "" {
		return "", nil, ErrInvalidToken
	}
	email := normalizeEmail(id.Email)
	if email == "" {
		return "", nil, ErrInvalidToken
	}

	user, err := s.Users.GetByGoogleSub(ctx, id.Subject)
	if err != nil {
		return "", nil, err
	}
	if user == nil {
		// Not a returning Google user: Google must be authoritative for this
		// address before we link it to an account or create one.
		if !googleAuthoritative(email, id.HostedDomain) {
			return "", nil, ErrInvalidToken
		}
		user, err = s.Users.GetByEmail(ctx, email)
		if err != nil {
			return "", nil, err
		}
		if user != nil {
			if user.GoogleSub != nil && *user.GoogleSub != id.Subject {
				return "", nil, ErrInvalidToken
			}
			sub := id.Subject
			user.GoogleSub = &sub
			if err := s.Users.Update(ctx, user); err != nil {
				return "", nil, err
			}
		}
	}
	if user == nil {
		username, err := s.UserSvc.GenerateUsername(ctx, email)
		if err != nil {
			return "", nil, err
		}
		sub := id.Subject
		user = &models.User{
			Email:       email,
			Username:    username,
			GoogleSub:   &sub,
			DisplayName: truncateRunes(id.Name, maxDisplayNameRunes),
			AvatarURL:   id.Picture,
		}
		if err := s.Users.Create(ctx, user); err != nil {
			return "", nil, err
		}
	}

	sessionToken, err := s.createSession(ctx, user.ID, s.Now())
	if err != nil {
		return "", nil, err
	}
	return sessionToken, user, nil
}

// googleAuthoritative reports whether Google can vouch for ownership of email:
// gmail.com addresses, or any address on a Google Workspace domain (hd claim).
func googleAuthoritative(email, hostedDomain string) bool {
	return strings.HasSuffix(email, "@gmail.com") || hostedDomain != ""
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func (s *AuthService) Authenticate(ctx context.Context, sessionToken string) (*models.User, error) {
	if sessionToken == "" {
		return nil, ErrUnauthorized
	}
	sess, err := s.Auth.GetSessionByHash(ctx, auth.HashToken(sessionToken))
	if err != nil {
		return nil, err
	}
	if sess == nil || !s.Now().Before(sess.ExpiresAt) {
		return nil, ErrUnauthorized
	}
	user, err := s.Users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUnauthorized
	}
	return user, nil
}

func (s *AuthService) Logout(ctx context.Context, sessionToken string) error {
	if sessionToken == "" {
		return nil
	}
	return s.Auth.DeleteSessionByHash(ctx, auth.HashToken(sessionToken))
}

// PurgeExpired deletes expired sessions and login tokens older than LoginTokenRetention.
func (s *AuthService) PurgeExpired(ctx context.Context) (sessions, tokens int64, err error) {
	now := s.Now()
	return s.Auth.DeleteExpired(ctx, now, now.Add(-LoginTokenRetention))
}
