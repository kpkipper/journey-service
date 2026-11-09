package repository

import (
	"context"
	"errors"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
)

type momentGorm struct {
	db *gorm.DB
}

func NewMomentRepository(db *gorm.DB) MomentRepository {
	return &momentGorm{db: db}
}

func (r *momentGorm) Create(ctx context.Context, m *models.Moment) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *momentGorm) Get(ctx context.Context, id string) (*models.Moment, error) {
	var m models.Moment
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *momentGorm) MarkReady(ctx context.Context, m *models.Moment) error {
	res := r.db.WithContext(ctx).Model(&models.Moment{}).
		Where("id = ? AND status = ?", m.ID, models.MomentPending).
		Updates(map[string]any{
			"status":      models.MomentReady,
			"width":       m.Width,
			"height":      m.Height,
			"duration_ms": m.DurationMs,
			"object_key":  m.ObjectKey,
			"thumb_key":   m.ThumbKey,
			"size_bytes":  m.SizeBytes,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *momentGorm) UpdateCaption(ctx context.Context, id, caption string) error {
	return r.db.WithContext(ctx).Model(&models.Moment{}).Where("id = ?", id).Update("caption", caption).Error
}

func (r *momentGorm) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Moment{}, "id = ?", id).Error
}

func (r *momentGorm) find(ctx context.Context, order string, where string, args ...any) ([]models.Moment, error) {
	var out []models.Moment
	err := r.db.WithContext(ctx).Where(where, args...).Order(order).Find(&out).Error
	return out, err
}

func (r *momentGorm) ListReadyByJourney(ctx context.Context, journeyID string) ([]models.Moment, error) {
	return r.find(ctx, "created_at, id", "journey_id = ? AND status = ?", journeyID, models.MomentReady)
}

func (r *momentGorm) ListByUploader(ctx context.Context, userID string) ([]models.Moment, error) {
	return r.find(ctx, "created_at DESC, id DESC", "uploader_id = ? AND status = ?", userID, models.MomentReady)
}

func (r *momentGorm) ListByJourney(ctx context.Context, journeyID string) ([]models.Moment, error) {
	return r.find(ctx, "created_at, id", "journey_id = ?", journeyID)
}

func (r *momentGorm) ListPendingBefore(ctx context.Context, t time.Time) ([]models.Moment, error) {
	return r.find(ctx, "created_at, id", "status = ? AND created_at < ?", models.MomentPending, t)
}

func (r *momentGorm) count(ctx context.Context, where string, args ...any) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Moment{}).Where(where, args...).Count(&n).Error
	return n, err
}

func (r *momentGorm) CountByJourney(ctx context.Context, journeyID string) (int64, error) {
	return r.count(ctx, "journey_id = ?", journeyID)
}

func (r *momentGorm) CountByDay(ctx context.Context, dayID string) (int64, error) {
	return r.count(ctx, "day_id = ?", dayID)
}

func (r *momentGorm) CountPendingByUploader(ctx context.Context, userID string) (int64, error) {
	return r.count(ctx, "uploader_id = ? AND status = ?", userID, models.MomentPending)
}

func (r *momentGorm) CountStartedSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	return r.count(ctx, "uploader_id = ? AND created_at >= ?", userID, since)
}

func (r *momentGorm) DetachPlan(ctx context.Context, planID string) error {
	return r.db.WithContext(ctx).Model(&models.Moment{}).Where("plan_id = ?", planID).Update("plan_id", "").Error
}
