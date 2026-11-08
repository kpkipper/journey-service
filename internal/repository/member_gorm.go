package repository

import (
	"context"
	"errors"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
)

type memberGorm struct {
	db *gorm.DB
}

func NewMemberRepository(db *gorm.DB) MemberRepository {
	return &memberGorm{db: db}
}

func (r *memberGorm) Create(ctx context.Context, m *models.JourneyMember) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *memberGorm) getMember(ctx context.Context, query string, args ...any) (*models.JourneyMember, error) {
	var m models.JourneyMember
	err := r.db.WithContext(ctx).First(&m, append([]any{query}, args...)...).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *memberGorm) Get(ctx context.Context, journeyID, userID string) (*models.JourneyMember, error) {
	return r.getMember(ctx, "journey_id = ? AND user_id = ?", journeyID, userID)
}

func (r *memberGorm) GetByID(ctx context.Context, id string) (*models.JourneyMember, error) {
	return r.getMember(ctx, "id = ?", id)
}

func (r *memberGorm) Update(ctx context.Context, m *models.JourneyMember) error {
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *memberGorm) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.JourneyMember{}, "id = ?", id).Error
}

func (r *memberGorm) ListByJourney(ctx context.Context, journeyID string) ([]models.MemberView, error) {
	var out []models.MemberView
	err := r.db.WithContext(ctx).
		Table("journey_members").
		Select("users.username AS username, users.display_name AS display_name, users.avatar_url AS avatar_url, journey_members.role AS role, journey_members.status AS status").
		Joins("JOIN users ON users.id = journey_members.user_id").
		Where("journey_members.journey_id = ?", journeyID).
		Order("CASE WHEN journey_members.status = 'joined' THEN 0 ELSE 1 END").
		Order("journey_members.created_at ASC").
		Scan(&out).Error
	return out, err
}

func (r *memberGorm) ListInvitesForUser(ctx context.Context, userID string) ([]models.JourneyMember, error) {
	var out []models.JourneyMember
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, models.StatusInvited).
		Order("created_at ASC").
		Find(&out).Error
	return out, err
}

func (r *memberGorm) ListJourneyIDsForUser(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).
		Model(&models.JourneyMember{}).
		Where("user_id = ? AND status = ?", userID, models.StatusJoined).
		Pluck("journey_id", &ids).Error
	return ids, err
}

func (r *memberGorm) CountActive(ctx context.Context, journeyID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&models.JourneyMember{}).
		Where("journey_id = ? AND status IN ?", journeyID, []string{models.StatusInvited, models.StatusJoined}).
		Count(&n).Error
	return n, err
}

func (r *memberGorm) CreateInviteLink(ctx context.Context, l *models.InviteLink) error {
	return r.db.WithContext(ctx).Create(l).Error
}

func (r *memberGorm) getLink(ctx context.Context, query string, args ...any) (*models.InviteLink, error) {
	var l models.InviteLink
	err := r.db.WithContext(ctx).First(&l, append([]any{query}, args...)...).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *memberGorm) GetInviteLinkByHash(ctx context.Context, hash string) (*models.InviteLink, error) {
	return r.getLink(ctx, "token_hash = ?", hash)
}

func (r *memberGorm) GetInviteLink(ctx context.Context, journeyID, id string) (*models.InviteLink, error) {
	return r.getLink(ctx, "journey_id = ? AND id = ?", journeyID, id)
}

func (r *memberGorm) RevokeInviteLink(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.InviteLink{}).
		Where("id = ?", id).
		Update("revoked_at", at).Error
}

func (r *memberGorm) ListInviteLinks(ctx context.Context, journeyID string) ([]models.InviteLink, error) {
	var out []models.InviteLink
	err := r.db.WithContext(ctx).
		Where("journey_id = ? AND revoked_at IS NULL", journeyID).
		Order("created_at ASC").
		Find(&out).Error
	return out, err
}
