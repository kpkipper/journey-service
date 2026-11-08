package services

import (
	"context"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccess(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	svc := NewAccessService(repository.NewJourneyRepository(db), repository.NewMemberRepository(db))

	for _, u := range []string{"owner", "edtr", "trav", "inv", "out"} {
		require.NoError(t, db.Create(&models.User{ID: u, Email: u + "@x.com", Username: u, DisplayName: u}).Error)
	}
	newJourney := func(ownerID string, editorsCanInvite bool) *models.Journey {
		j := &models.Journey{
			OwnerID: ownerID, Title: "t", Destination: "Nagoya",
			DepartureDate: time.Now(), ReturnDate: time.Now().Add(24 * time.Hour),
			EditorsCanInvite: editorsCanInvite,
		}
		require.NoError(t, db.Create(j).Error)
		return j
	}
	addMember := func(j *models.Journey, userID, role, status string) {
		require.NoError(t, db.Create(&models.JourneyMember{JourneyID: j.ID, UserID: userID, Role: role, Status: status}).Error)
	}

	closed := newJourney("owner", false)
	addMember(closed, "edtr", models.RoleEditor, models.StatusJoined)
	addMember(closed, "trav", models.RoleTraveler, models.StatusJoined)
	addMember(closed, "inv", models.RoleEditor, models.StatusInvited)

	open := newJourney("owner", true)
	addMember(open, "edtr", models.RoleEditor, models.StatusJoined)

	legacy := newJourney("", true)

	tests := []struct {
		name               string
		slug, userID, role string
		canEdit, canInvite bool
		owner, member      bool
	}{
		{"owner", closed.Slug, "owner", "owner", true, true, true, true},
		{"editor cannot invite when disabled", closed.Slug, "edtr", "editor", true, false, false, true},
		{"editor can invite when enabled", open.Slug, "edtr", "editor", true, true, false, true},
		{"traveler", closed.Slug, "trav", "traveler", false, false, false, true},
		{"invited editor has no access", closed.Slug, "inv", "", false, false, false, false},
		{"outsider", closed.Slug, "out", "", false, false, false, false},
		{"anonymous", closed.Slug, "", "", false, false, false, false},
		{"legacy trip anonymous", legacy.Slug, "", "", false, false, false, false},
		{"legacy trip signed-in user", legacy.Slug, "out", "", false, false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := svc.Load(ctx, tc.slug, tc.userID)
			require.NoError(t, err)
			require.NotNil(t, a.Journey)
			assert.Equal(t, tc.role, a.Role)
			assert.Equal(t, tc.canEdit, a.CanEdit(), "CanEdit")
			assert.Equal(t, tc.canInvite, a.CanInvite(), "CanInvite")
			assert.Equal(t, tc.owner, a.IsOwner(), "IsOwner")
			assert.Equal(t, tc.member, a.IsMember(), "IsMember")
		})
	}

	t.Run("missing trip", func(t *testing.T) {
		_, err := svc.Load(ctx, "nope", "owner")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}
