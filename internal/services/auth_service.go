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
)

const (
	SessionTTL           = 30 * 24 * time.Hour
	LoginTokenTTL        = 15 * time.Minute
	MaxLoginLinksPerHour = 5
)

var (
	ErrInvalidToken    = errors.New("invalid or expired token")
	ErrTooManyRequests = errors.New("too many requests")
	ErrUnauthorized    = errors.New("unauthorized")
)

type AuthDeps struct {
	Users   repository.UserRepository
	Auth    repository.AuthRepository
	UserSvc *UserService
	Mailer  mailer.Mailer
	WebURL  string
	Now     func() time.Time // nil = time.Now
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

func (s *AuthService) StartEmailLogin(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
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
