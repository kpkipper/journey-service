package repository

import (
	"context"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type blockGorm struct {
	db *gorm.DB
}

func NewBlockRepository(db *gorm.DB) BlockRepository {
	return &blockGorm{db: db}
}

func insertBlock(db *gorm.DB, blockerID, blockedID string) error {
	return db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.Block{BlockerID: blockerID, BlockedID: blockedID}).Error
}

func (r *blockGorm) Block(ctx context.Context, blockerID, blockedID string) error {
	return insertBlock(r.db.WithContext(ctx), blockerID, blockedID)
}

func (r *blockGorm) BlockAndUnfollow(ctx context.Context, blockerID, blockedID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := insertBlock(tx, blockerID, blockedID); err != nil {
			return err
		}
		// Requests before follows: a concurrent accept holds the request row
		// lock, so we wait for it and then delete the follow it committed.
		if err := tx.Where("(requester_id = ? AND target_id = ?) OR (requester_id = ? AND target_id = ?)",
			blockerID, blockedID, blockedID, blockerID).Delete(&models.FollowRequest{}).Error; err != nil {
			return err
		}
		if err := tx.Where("(follower_id = ? AND followee_id = ?) OR (follower_id = ? AND followee_id = ?)",
			blockerID, blockedID, blockedID, blockerID).Delete(&models.Follow{}).Error; err != nil {
			return err
		}
		// Pending invites between the two (in trips either owns, or sent by the
		// other) die with the block; joined memberships stay.
		owned := tx.Model(&models.Journey{}).Select("id").Where("owner_id = ?", blockerID)
		ownedByBlocked := tx.Model(&models.Journey{}).Select("id").Where("owner_id = ?", blockedID)
		return tx.Where("status = ? AND ((user_id = ? AND (journey_id IN (?) OR invited_by = ?)) OR (user_id = ? AND (journey_id IN (?) OR invited_by = ?)))",
			models.StatusInvited, blockedID, owned, blockerID, blockerID, ownedByBlocked, blockedID).
			Delete(&models.JourneyMember{}).Error
	})
}

func (r *blockGorm) Unblock(ctx context.Context, blockerID, blockedID string) error {
	return r.db.WithContext(ctx).
		Where("blocker_id = ? AND blocked_id = ?", blockerID, blockedID).
		Delete(&models.Block{}).Error
}

func (r *blockGorm) Blocked(ctx context.Context, a, b string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("(blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)", a, b, b, a).
		Count(&n).Error
	return n > 0, err
}

func (r *blockGorm) ListBlocked(ctx context.Context, blockerID string) ([]models.PublicProfile, error) {
	out := []models.PublicProfile{}
	err := r.db.WithContext(ctx).Table("blocks").
		Select("users.username, users.display_name, users.avatar_url, users.bio").
		Joins("JOIN users ON users.id = blocks.blocked_id").
		Where("blocks.blocker_id = ?", blockerID).
		Order("blocks.created_at DESC, users.username ASC").
		Scan(&out).Error
	return out, err
}

func (r *blockGorm) RelatedIDs(ctx context.Context, userID string) (map[string]bool, error) {
	var rows []models.Block
	err := r.db.WithContext(ctx).
		Where("blocker_id = ? OR blocked_id = ?", userID, userID).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, b := range rows {
		if b.BlockerID == userID {
			out[b.BlockedID] = true
		} else {
			out[b.BlockerID] = true
		}
	}
	return out, nil
}

func (r *blockGorm) IsBlocking(ctx context.Context, blockerID, blockedID string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Block{}).
		Where("blocker_id = ? AND blocked_id = ?", blockerID, blockedID).Count(&n).Error
	return n > 0, err
}
