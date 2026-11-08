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
// the given username and returns how many journeys were updated.
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

	// The soft-delete scope on models.Journey excludes deleted rows.
	res := db.WithContext(ctx).Model(&models.Journey{}).
		Where("owner_id IS NULL OR owner_id = ''").
		Update("owner_id", user.ID)
	return res.RowsAffected, res.Error
}
