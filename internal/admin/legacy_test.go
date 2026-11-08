package admin_test

import (
	"context"
	"testing"

	"github.com/kpkipper/journey-service/internal/admin"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignLegacyOwner(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	alice := models.User{Email: "alice@example.com", Username: "alice"}
	bob := models.User{Email: "bob@example.com", Username: "bob"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	emptyOwner := models.Journey{Title: "empty", Destination: "Tokyo", OwnerID: ""}
	nullOwner := models.Journey{Title: "null", Destination: "Kyoto"}
	bobs := models.Journey{Title: "bobs", Destination: "Nagoya", OwnerID: bob.ID}
	deleted := models.Journey{Title: "deleted", Destination: "Osaka", OwnerID: ""}
	for _, j := range []*models.Journey{&emptyOwner, &nullOwner, &bobs, &deleted} {
		require.NoError(t, db.Create(j).Error)
	}
	// Force a real NULL owner and soft-delete one legacy trip.
	require.NoError(t, db.Exec("UPDATE journeys SET owner_id = NULL WHERE id = ?", nullOwner.ID).Error)
	require.NoError(t, db.Delete(&deleted).Error)

	n, err := admin.AssignLegacyOwner(ctx, db, " Alice ")
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	ownerOf := func(id string) string {
		var owner *string
		require.NoError(t, db.Unscoped().Model(&models.Journey{}).Where("id = ?", id).Select("owner_id").Row().Scan(&owner))
		if owner == nil {
			return "<null>"
		}
		return *owner
	}
	assert.Equal(t, alice.ID, ownerOf(emptyOwner.ID))
	assert.Equal(t, alice.ID, ownerOf(nullOwner.ID))
	assert.Equal(t, bob.ID, ownerOf(bobs.ID))
	assert.Equal(t, "", ownerOf(deleted.ID), "soft-deleted trips are untouched")

	n, err = admin.AssignLegacyOwner(ctx, db, "alice")
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestAssignLegacyOwnerUnknownUser(t *testing.T) {
	db := testutil.NewTestDB(t)
	n, err := admin.AssignLegacyOwner(context.Background(), db, "nobody")
	require.EqualError(t, err, "user not found")
	assert.Equal(t, int64(0), n)
}
