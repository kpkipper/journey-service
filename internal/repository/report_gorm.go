package repository

import (
	"context"
	"errors"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
)

type reportGorm struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) ReportRepository {
	return &reportGorm{db: db}
}

func (r *reportGorm) Create(ctx context.Context, rep *models.Report) error {
	return r.db.WithContext(ctx).Create(rep).Error
}

func (r *reportGorm) FindOpen(ctx context.Context, reporterID, targetType, targetID string) (*models.Report, error) {
	var rep models.Report
	err := r.db.WithContext(ctx).
		Where("reporter_id = ? AND target_type = ? AND target_id = ? AND status = ?",
			reporterID, targetType, targetID, models.ReportOpen).
		Order("created_at ASC").First(&rep).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rep, nil
}

func (r *reportGorm) CountSince(ctx context.Context, reporterID string, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Report{}).
		Where("reporter_id = ? AND created_at >= ?", reporterID, since).Count(&n).Error
	return n, err
}

func (r *reportGorm) ListOpen(ctx context.Context) ([]models.Report, error) {
	out := []models.Report{}
	err := r.db.WithContext(ctx).Where("status = ?", models.ReportOpen).
		Order("created_at ASC, id ASC").Find(&out).Error
	return out, err
}

func (r *reportGorm) Get(ctx context.Context, id string) (*models.Report, error) {
	var rep models.Report
	err := r.db.WithContext(ctx).First(&rep, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rep, nil
}

func (r *reportGorm) Resolve(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&models.Report{}).
		Where("id = ? AND status = ?", id, models.ReportOpen).
		Updates(map[string]any{"status": models.ReportResolved, "resolved_at": at}).Error
}
