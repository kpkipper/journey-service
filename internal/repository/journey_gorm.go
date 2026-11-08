package repository

import (
	"context"
	"errors"

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
		Order("created_at DESC, id DESC").
		Find(&journeys).Error
	return journeys, err
}

func (r *journeyGorm) GetByID(ctx context.Context, id string) (*models.Journey, error) {
	var journey models.Journey
	err := r.db.WithContext(ctx).First(&journey, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &journey, nil
}

func (r *journeyGorm) GetBySlug(ctx context.Context, slug string) (*models.Journey, error) {
	var journey models.Journey
	err := r.db.WithContext(ctx).
		Preload("ItineraryDays", func(db *gorm.DB) *gorm.DB {
			return db.Order("date_iso ASC")
		}).
		Preload("ItineraryDays.Plans", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC, time ASC")
		}).
		First(&journey, "slug = ?", slug).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &journey, err
}

func (r *journeyGorm) Update(ctx context.Context, journey *models.Journey) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Only the full-PUT columns; cover_url, editors_can_invite, owner, slug and
		// deleted_at are never written here, and a missing/soft-deleted row is not resurrected.
		res := tx.Model(&models.Journey{}).Where("id = ?", journey.ID).
			Select("title", "destination", "country", "departure_date", "return_date").
			Updates(journey)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
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

func (r *journeyGorm) Delete(ctx context.Context, slug string) error {
	result := r.db.WithContext(ctx).Where("slug = ?", slug).Delete(&models.Journey{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *journeyGorm) ListByOwner(ctx context.Context, ownerID string) ([]models.Journey, error) {
	var journeys []models.Journey
	err := r.db.WithContext(ctx).Where("owner_id = ?", ownerID).Find(&journeys).Error
	return journeys, err
}

func (r *journeyGorm) ListByIDs(ctx context.Context, ids []string) ([]models.Journey, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var journeys []models.Journey
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&journeys).Error
	return journeys, err
}
