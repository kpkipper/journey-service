package repository

import (
	"context"
	"errors"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
)

type itineraryGorm struct {
	db *gorm.DB
}

func NewItineraryRepository(db *gorm.DB) ItineraryRepository {
	return &itineraryGorm{db: db}
}

func (r *itineraryGorm) GetDay(ctx context.Context, journeyID, dayID string) (*models.ItineraryDay, error) {
	var d models.ItineraryDay
	err := r.db.WithContext(ctx).First(&d, "id = ? AND journey_id = ?", dayID, journeyID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *itineraryGorm) CreateDay(ctx context.Context, d *models.ItineraryDay) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *itineraryGorm) UpdateDay(ctx context.Context, d *models.ItineraryDay) error {
	return updateOne(r.db.WithContext(ctx).Model(&models.ItineraryDay{}).Where("id = ?", d.ID),
		map[string]any{"date": d.Date, "date_iso": d.DateISO, "title": d.Title})
}

// DeleteDay removes the day and its plans explicitly (SQLite may not enforce FK cascades).
func (r *itineraryGorm) DeleteDay(ctx context.Context, d *models.ItineraryDay) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("itinerary_day_id = ?", d.ID).Delete(&models.ActivityPlan{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND journey_id = ?", d.ID, d.JourneyID).Delete(&models.ItineraryDay{}).Error
	})
}

func (r *itineraryGorm) GetPlan(ctx context.Context, journeyID, planID string) (*models.ActivityPlan, error) {
	var p models.ActivityPlan
	err := r.db.WithContext(ctx).
		Joins("JOIN itinerary_days ON itinerary_days.id = activity_plans.itinerary_day_id").
		Where("activity_plans.id = ? AND itinerary_days.journey_id = ?", planID, journeyID).
		First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *itineraryGorm) CreatePlan(ctx context.Context, p *models.ActivityPlan) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *itineraryGorm) UpdatePlan(ctx context.Context, p *models.ActivityPlan) error {
	return updateOne(r.db.WithContext(ctx).Model(&models.ActivityPlan{}).Where("id = ?", p.ID), map[string]any{
		"itinerary_day_id": p.ItineraryDayID, "time": p.Time, "description": p.Description,
		"emoji": p.Emoji, "map_url": p.MapURL, "position": p.Position,
	})
}

func (r *itineraryGorm) DeletePlan(ctx context.Context, planID string) error {
	return r.db.WithContext(ctx).Where("id = ?", planID).Delete(&models.ActivityPlan{}).Error
}

func (r *itineraryGorm) ListPlanIDs(ctx context.Context, dayID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&models.ActivityPlan{}).
		Where("itinerary_day_id = ?", dayID).Order("position ASC").Pluck("id", &ids).Error
	return ids, err
}

func (r *itineraryGorm) MaxPosition(ctx context.Context, dayID string) (int, error) {
	var max int
	err := r.db.WithContext(ctx).Model(&models.ActivityPlan{}).
		Where("itinerary_day_id = ?", dayID).
		Select("COALESCE(MAX(position), -1)").Scan(&max).Error
	return max, err
}

func (r *itineraryGorm) SetPositions(ctx context.Context, dayID string, ids []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			res := tx.Model(&models.ActivityPlan{}).
				Where("id = ? AND itinerary_day_id = ?", id, dayID).Update("position", i)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
}

func (r *itineraryGorm) UpdateJourneyFields(ctx context.Context, journeyID string, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.Journey{}).Where("id = ?", journeyID).Updates(fields).Error
}

// updateOne updates only the given columns and reports gorm.ErrRecordNotFound
// when the row no longer exists (so a deleted row is never resurrected).
func updateOne(q *gorm.DB, fields map[string]any) error {
	res := q.Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
