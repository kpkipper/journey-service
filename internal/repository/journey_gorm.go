package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
)

type journeyGorm struct {
	db *gorm.DB
}

func NewJourneyRepository(db *gorm.DB) JourneyRepository {
	return &journeyGorm{db: db}
}

func (r *journeyGorm) Create(ctx context.Context, journey *models.Journey) error {
	return r.db.WithContext(ctx).Create(journey).Error
}

func (r *journeyGorm) List(ctx context.Context) ([]models.Journey, error) {
	var journeys []models.Journey
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Find(&journeys).Error
	return journeys, err
}

func (r *journeyGorm) GetByID(ctx context.Context, id uuid.UUID) (*models.Journey, error) {
	var journey models.Journey
	err := r.db.WithContext(ctx).
		Preload("ItineraryDays", func(db *gorm.DB) *gorm.DB {
			return db.Order("date_iso ASC")
		}).
		Preload("ItineraryDays.Plans", func(db *gorm.DB) *gorm.DB {
			return db.Order("time ASC")
		}).
		First(&journey, "id = ?", id).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &journey, err
}

func (r *journeyGorm) Update(ctx context.Context, journey *models.Journey) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(journey).Error; err != nil {
			return err
		}

		// Delete existing days (cascade deletes plans)
		if err := tx.Where("journey_id = ?", journey.ID).Delete(&models.ItineraryDay{}).Error; err != nil {
			return err
		}

		// Re-create days and plans
		for i := range journey.ItineraryDays {
			journey.ItineraryDays[i].JourneyID = journey.ID
			if err := tx.Create(&journey.ItineraryDays[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *journeyGorm) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&models.Journey{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *journeyGorm) DeleteAll(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("1 = 1").Delete(&models.Journey{}).Error
}
