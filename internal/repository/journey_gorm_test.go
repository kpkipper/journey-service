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

func TestJourneyRepository_PlanOrderByPosition(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewJourneyRepository(db)
	ctx := context.Background()
	dep := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	mk := func(p1, p2 int) *models.Journey {
		return &models.Journey{
			Title: "T", Destination: "Tokyo", DepartureDate: dep, ReturnDate: dep.AddDate(0, 0, 1),
			ItineraryDays: []models.ItineraryDay{{
				Date: "1 Dec", DateISO: dep, Title: "Day 1",
				Plans: []models.ActivityPlan{
					{Time: "09:00", Description: "A", Position: p1},
					{Time: "18:00", Description: "B", Position: p2},
				},
			}},
		}
	}

	j := mk(1, 0)
	require.NoError(t, repo.Create(ctx, j))
	got, err := repo.GetBySlug(ctx, j.Slug)
	require.NoError(t, err)
	require.Equal(t, "18:00", got.ItineraryDays[0].Plans[0].Time)
	require.Equal(t, "09:00", got.ItineraryDays[0].Plans[1].Time)

	j2 := mk(0, 0)
	require.NoError(t, repo.Create(ctx, j2))
	got, err = repo.GetBySlug(ctx, j2.Slug)
	require.NoError(t, err)
	require.Equal(t, "09:00", got.ItineraryDays[0].Plans[0].Time)
}

func TestJourneyRepository_ListByOwnerAndIDs(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewJourneyRepository(db)
	ctx := context.Background()
	dep := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	mk := func(owner string) *models.Journey {
		j := &models.Journey{Title: "T", Destination: "Tokyo", OwnerID: owner, DepartureDate: dep, ReturnDate: dep}
		require.NoError(t, repo.Create(ctx, j))
		return j
	}
	a1, _, b1 := mk("userA"), mk("userA"), mk("userB")

	owned, err := repo.ListByOwner(ctx, "userA")
	require.NoError(t, err)
	require.Len(t, owned, 2)

	byIDs, err := repo.ListByIDs(ctx, []string{a1.ID, b1.ID})
	require.NoError(t, err)
	require.Len(t, byIDs, 2)

	none, err := repo.ListByIDs(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, none)
}
