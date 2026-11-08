package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestJourneyRepository_CreateAndGetBySlug(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewJourneyRepository(db)
	ctx := context.Background()

	dep := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	j := &models.Journey{
		Title:         "Japan Trip",
		Destination:   "Tokyo, Japan",
		DepartureDate: dep,
		ReturnDate:    dep.AddDate(0, 0, 5),
		ItineraryDays: []models.ItineraryDay{{
			Date:    "1 Dec",
			DateISO: dep,
			Title:   "Day 1",
			Plans: []models.ActivityPlan{
				{Time: "19:00", Description: "Dinner"},
				{Time: "14:00", Description: "Temple"},
			},
		}},
	}
	require.NoError(t, repo.Create(ctx, j))
	require.Equal(t, "tokyo-japan-"+j.ID, j.Slug)

	got, err := repo.GetBySlug(ctx, j.Slug)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.ItineraryDays, 1)
	require.Len(t, got.ItineraryDays[0].Plans, 2)
	require.Equal(t, "14:00", got.ItineraryDays[0].Plans[0].Time)
	require.Equal(t, "19:00", got.ItineraryDays[0].Plans[1].Time)

	missing, err := repo.GetBySlug(ctx, "nope")
	require.NoError(t, err)
	require.Nil(t, missing)
}
