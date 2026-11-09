package repository

import (
	"context"
	"errors"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
)

type userGorm struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userGorm{db: db}
}

func (r *userGorm) Create(ctx context.Context, u *models.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *userGorm) getWhere(ctx context.Context, query string, arg any) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).First(&u, query, arg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userGorm) GetByID(ctx context.Context, id string) (*models.User, error) {
	return r.getWhere(ctx, "id = ?", id)
}

func (r *userGorm) GetByIDs(ctx context.Context, ids []string) ([]models.User, error) {
	var out []models.User
	if len(ids) == 0 {
		return out, nil
	}
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&out).Error
	return out, err
}

func (r *userGorm) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	return r.getWhere(ctx, "email = ?", email)
}

func (r *userGorm) GetByGoogleSub(ctx context.Context, sub string) (*models.User, error) {
	return r.getWhere(ctx, "google_sub = ?", sub)
}

func (r *userGorm) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	return r.getWhere(ctx, "username = ?", username)
}

// Update saves the whole row except follow_approval, which only
// SetFollowApproval writes (a stale struct must not undo a concurrent toggle).
func (r *userGorm) Update(ctx context.Context, u *models.User) error {
	return r.db.WithContext(ctx).Omit("follow_approval").Save(u).Error
}

func (r *userGorm) SetFollowApproval(ctx context.Context, userID string, on bool) error {
	return r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).
		Update("follow_approval", on).Error
}
