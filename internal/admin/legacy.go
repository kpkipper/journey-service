// Package admin holds one-off operator commands run from the CLI.
package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/kpkipper/journey-service/internal/models"
	"gorm.io/gorm"
)

// AssignLegacyOwner gives every ownerless, non-deleted journey to the user with
// the given username, makes those trips public and hides their hotel plans
// (emoji 🏨 or category stay) from non-members. It returns how many journeys
// were claimed.
func AssignLegacyOwner(ctx context.Context, db *gorm.DB, username string) (int64, error) {
	username = strings.ToLower(strings.TrimSpace(username))

	var user models.User
	err := db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, errors.New("user not found")
	}
	if err != nil {
		return 0, err
	}

	var claimed int64
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The soft-delete scope on models.Journey excludes deleted rows.
		var ids []string
		if err := tx.Model(&models.Journey{}).
			Where("owner_id IS NULL OR owner_id = ''").
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		res := tx.Model(&models.Journey{}).Where("id IN ?", ids).
			Updates(map[string]any{"owner_id": user.ID, "visibility": models.VisibilityPublic})
		if res.Error != nil {
			return res.Error
		}
		claimed = res.RowsAffected

		dayIDs := tx.Model(&models.ItineraryDay{}).Select("id").Where("journey_id IN ?", ids)
		return tx.Model(&models.ActivityPlan{}).
			Where("itinerary_day_id IN (?)", dayIDs).
			Where("emoji = ? OR category = ?", "🏨", "stay").
			Update("hidden_from_public", true).Error
	})
	return claimed, err
}
