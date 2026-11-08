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

func TestAssignLegacyOwnerPublicAndHidesHotels(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	alice := models.User{Email: "alice@example.com", Username: "alice"}
	bob := models.User{Email: "bob@example.com", Username: "bob"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	mk := func(owner, dest string) (models.Journey, []models.ActivityPlan) {
		j := models.Journey{Title: dest, Destination: dest, OwnerID: owner, Visibility: models.VisibilityPrivate}
		require.NoError(t, db.Create(&j).Error)
		d := models.ItineraryDay{JourneyID: j.ID, Date: "d1"}
		require.NoError(t, db.Create(&d).Error)
		plans := []models.ActivityPlan{
			{ItineraryDayID: d.ID, Description: "hotel emoji", Emoji: "🏨"},
			{ItineraryDayID: d.ID, Description: "stay cat", Category: "stay"},
			{ItineraryDayID: d.ID, Description: "ramen", Emoji: "🍜", Category: "food"},
		}
		for i := range plans {
			require.NoError(t, db.Create(&plans[i]).Error)
		}
		return j, plans
	}
	legacy, legacyPlans := mk("", "Tokyo")
	bobs, bobPlans := mk(bob.ID, "Nagoya")

	n, err := admin.AssignLegacyOwner(ctx, db, "alice")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	var got models.Journey
	require.NoError(t, db.First(&got, "id = ?", legacy.ID).Error)
	assert.Equal(t, alice.ID, got.OwnerID)
	assert.Equal(t, models.VisibilityPublic, got.Visibility)
	var bobGot models.Journey
	require.NoError(t, db.First(&bobGot, "id = ?", bobs.ID).Error)
	assert.Equal(t, models.VisibilityPrivate, bobGot.Visibility, "bob's trip untouched")

	hidden := func(p models.ActivityPlan) bool {
		var cur models.ActivityPlan
		require.NoError(t, db.First(&cur, "id = ?", p.ID).Error)
		return cur.HiddenFromPublic
	}
	assert.True(t, hidden(legacyPlans[0]), "hotel emoji plan hidden")
	assert.True(t, hidden(legacyPlans[1]), "stay plan hidden")
	assert.False(t, hidden(legacyPlans[2]), "food plan stays visible")
	for _, p := range bobPlans {
		assert.False(t, hidden(p), "bob's plans untouched")
	}

	n, err = admin.AssignLegacyOwner(ctx, db, "alice")
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}
