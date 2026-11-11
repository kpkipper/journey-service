package repository

import (
	"context"
	"errors"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
)

type authGorm struct {
	db *gorm.DB
}

func NewAuthRepository(db *gorm.DB) AuthRepository {
	return &authGorm{db: db}
}

func (r *authGorm) CreateSession(ctx context.Context, s *models.Session) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *authGorm) GetSessionByHash(ctx context.Context, hash string) (*models.Session, error) {
	var s models.Session
	err := r.db.WithContext(ctx).First(&s, "token_hash = ?", hash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *authGorm) DeleteSessionByHash(ctx context.Context, hash string) error {
	return r.db.WithContext(ctx).Where("token_hash = ?", hash).Delete(&models.Session{}).Error
}

func (r *authGorm) CreateLoginToken(ctx context.Context, t *models.LoginToken) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *authGorm) GetLoginTokenByHash(ctx context.Context, hash string) (*models.LoginToken, error) {
	var t models.LoginToken
	err := r.db.WithContext(ctx).First(&t, "token_hash = ?", hash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *authGorm) MarkLoginTokenUsed(ctx context.Context, id string, at time.Time) (bool, error) {
	res := r.db.WithContext(ctx).
		Model(&models.LoginToken{}).
		Where("id = ? AND used_at IS NULL", id).
		Update("used_at", at)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (r *authGorm) CountLoginTokensSince(ctx context.Context, email string, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&models.LoginToken{}).
		Where("email = ? AND created_at >= ?", email, since).
		Count(&n).Error
	return n, err
}

func (r *authGorm) CountAllLoginTokensSince(ctx context.Context, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&models.LoginToken{}).
		Where("created_at >= ?", since).
		Count(&n).Error
	return n, err
}

func (r *authGorm) DeleteExpired(ctx context.Context, now, tokensBefore time.Time) (int64, int64, error) {
	s := r.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&models.Session{})
	if s.Error != nil {
		return 0, 0, s.Error
	}
	t := r.db.WithContext(ctx).Where("created_at < ?", tokensBefore).Delete(&models.LoginToken{})
	if t.Error != nil {
		return s.RowsAffected, 0, t.Error
	}
	return s.RowsAffected, t.RowsAffected, nil
}
