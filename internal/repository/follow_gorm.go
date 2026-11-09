package repository

import (
	"context"
	"time"

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

func (r *followGorm) Request(ctx context.Context, requesterID, targetID string) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.FollowRequest{RequesterID: requesterID, TargetID: targetID}).Error
}

func (r *followGorm) CancelRequest(ctx context.Context, requesterID, targetID string) error {
	return r.db.WithContext(ctx).
		Where("requester_id = ? AND target_id = ?", requesterID, targetID).
		Delete(&models.FollowRequest{}).Error
}

func (r *followGorm) HasRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.FollowRequest{}).
		Where("requester_id = ? AND target_id = ?", requesterID, targetID).Count(&n).Error
	return n > 0, err
}

func (r *followGorm) CountOutgoingRequests(ctx context.Context, requesterID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.FollowRequest{}).
		Where("requester_id = ?", requesterID).Count(&n).Error
	return n, err
}

func (r *followGorm) AcceptRequest(ctx context.Context, requesterID, targetID string) (bool, error) {
	accepted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("requester_id = ? AND target_id = ?", requesterID, targetID).
			Delete(&models.FollowRequest{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		// A block that raced the request wins: the request is gone, no follow.
		blocked, err := blockedTx(tx, requesterID, targetID)
		if err != nil || blocked {
			return err
		}
		accepted = true
		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&models.Follow{FollowerID: requesterID, FolloweeID: targetID}).Error
	})
	return accepted, err
}

// blockedTx reports whether a block exists between a and b in either direction.
func blockedTx(tx *gorm.DB, a, b string) (bool, error) {
	var n int64
	err := tx.Model(&models.Block{}).
		Where("(blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)", a, b, b, a).
		Count(&n).Error
	return n > 0, err
}

// notBlockedWithTarget restricts follow_requests rows to requesters with no
// block (either direction) against the row's target.
const notBlockedWithTarget = "NOT EXISTS (SELECT 1 FROM blocks WHERE " +
	"(blocks.blocker_id = follow_requests.requester_id AND blocks.blocked_id = follow_requests.target_id) OR " +
	"(blocks.blocker_id = follow_requests.target_id AND blocks.blocked_id = follow_requests.requester_id))"

// AcceptAllRequests deletes every request to targetID and turns those from
// requesters not in skip and not block-related into follows. Requests are
// deleted (and row-locked) before blocks are read, the same order as
// AcceptRequest, so a racing BlockAndUnfollow either removes the request first
// or removes the new follow after.
func (r *followGorm) AcceptAllRequests(ctx context.Context, targetID string, skip map[string]bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var reqs []models.FollowRequest
		if err := tx.Clauses(clause.Returning{Columns: []clause.Column{{Name: "requester_id"}}}).
			Where("target_id = ?", targetID).Delete(&reqs).Error; err != nil {
			return err
		}
		if len(reqs) == 0 {
			return nil
		}
		var blocks []models.Block
		if err := tx.Where("blocker_id = ? OR blocked_id = ?", targetID, targetID).
			Find(&blocks).Error; err != nil {
			return err
		}
		related := make(map[string]bool, len(blocks))
		for _, b := range blocks {
			related[b.BlockerID], related[b.BlockedID] = true, true
		}
		follows := make([]models.Follow, 0, len(reqs))
		for _, q := range reqs {
			if skip[q.RequesterID] || related[q.RequesterID] {
				continue
			}
			follows = append(follows, models.Follow{FollowerID: q.RequesterID, FolloweeID: targetID})
		}
		if len(follows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&follows).Error
	})
}

func (r *followGorm) ListRequests(ctx context.Context, targetID string, limit int) ([]models.FollowEntry, error) {
	return r.listEntries(ctx, "follow_requests", "requester_id", "target_id", targetID, limit)
}

func (r *followGorm) CountRequests(ctx context.Context, targetID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.FollowRequest{}).
		Where("target_id = ?", targetID).Where(notBlockedWithTarget).Count(&n).Error
	return n, err
}

func (r *followGorm) ListFollowers(ctx context.Context, userID string, limit int) ([]models.FollowEntry, error) {
	return r.listEntries(ctx, "follows", "follower_id", "followee_id", userID, limit)
}

type followEntryRow struct {
	ID          string
	Username    string
	DisplayName string
	AvatarURL   string
	Bio         string
	CreatedAt   time.Time
}

// listEntries joins table (the relation) to users on userCol and returns the
// rows where whereCol = id, newest first. Always a non-nil slice.
func (r *followGorm) listEntries(ctx context.Context, table, userCol, whereCol, id string, limit int) ([]models.FollowEntry, error) {
	var rows []followEntryRow
	q := r.db.WithContext(ctx).Table(table).
		Select("users.id, users.username, users.display_name, users.avatar_url, users.bio, "+table+".created_at").
		Joins("JOIN users ON users.id = "+table+"."+userCol).
		Where(table+"."+whereCol+" = ?", id).
		Order(table + ".created_at DESC, users.username ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.FollowEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, models.FollowEntry{
			UserID:    row.ID,
			User:      models.PublicProfile{Username: row.Username, DisplayName: row.DisplayName, AvatarURL: row.AvatarURL, Bio: row.Bio},
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}
