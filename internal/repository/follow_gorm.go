package repository

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type followGorm struct {
	db *gorm.DB
}

func NewFollowRepository(db *gorm.DB) FollowRepository {
	return &followGorm{db: db}
}

func (r *followGorm) Follow(ctx context.Context, followerID, followeeID string) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.Follow{FollowerID: followerID, FolloweeID: followeeID}).Error
}

func (r *followGorm) Unfollow(ctx context.Context, followerID, followeeID string) error {
	return r.db.WithContext(ctx).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		Delete(&models.Follow{}).Error
}

func (r *followGorm) IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Follow{}).
		Where("follower_id = ? AND followee_id = ?", followerID, followeeID).
		Count(&n).Error
	return n > 0, err
}

func (r *followGorm) CountFollowers(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Follow{}).Where("followee_id = ?", userID).Count(&n).Error
	return n, err
}

func (r *followGorm) CountFollowing(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Follow{}).Where("follower_id = ?", userID).Count(&n).Error
	return n, err
}

func (r *followGorm) ListFolloweeIDs(ctx context.Context, followerID string) ([]string, error) {
	ids := []string{}
	err := r.db.WithContext(ctx).Model(&models.Follow{}).
		Where("follower_id = ?", followerID).Pluck("followee_id", &ids).Error
	return ids, err
}
