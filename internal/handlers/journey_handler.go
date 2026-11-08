package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type JourneyHandler struct {
	svc     *services.JourneyService
	privacy *services.PrivacyService
}

func NewJourneyHandler(svc *services.JourneyService, privacy *services.PrivacyService) *JourneyHandler {
	return &JourneyHandler{svc: svc, privacy: privacy}
}

func (h *JourneyHandler) Create(c *fiber.Ctx) error {
	var journey models.Journey
	if err := c.BodyParser(&journey); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if journey.Title == "" || journey.Destination == "" {
		return utils.Error(c, fiber.StatusBadRequest, "title and destination are required")
	}

	if err := h.svc.Create(c.Context(), middleware.CurrentUserID(c), &journey); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, fiber.Map{"slug": journey.Slug})
}

// List groups the trips the viewer may see in lists by country, newest created
// first. Items are built from the viewer's JourneyView, so a redacted trip never
// carries dates (not even through the title).
func (h *JourneyHandler) List(c *fiber.Ctx) error {
	viewerID := middleware.CurrentUserID(c)
	journeys, err := h.svc.List(c.Context())
	if err != nil {
		return respondError(c, err)
	}

	groupedMap := map[string]*models.JourneyByCountry{}
	order := []string{}
	for i := range journeys {
		j := &journeys[i]
		v, ok, err := h.privacy.ViewOf(c.Context(), viewerID, j)
		if err != nil {
			return respondError(c, err)
		}
		if !ok {
			continue
		}
		rel, err := h.privacy.Relation(c.Context(), viewerID, j, v.Role)
		if err != nil {
			return respondError(c, err)
		}
		if !h.privacy.Listable(rel, j) {
			continue
		}
		country := v.Country
		if country == "" {
			country = "Other"
		}
		if _, ok := groupedMap[country]; !ok {
			groupedMap[country] = &models.JourneyByCountry{
				Country: country,
				Plan:    []models.JourneyListItem{},
			}
			order = append(order, country)
		}
		title := v.Title
		if !v.Redacted {
			title = fmt.Sprintf("%s %d", v.Destination, v.DepartureDate.Year())
		} else {
			title = v.Destination
		}
		groupedMap[country].Plan = append(groupedMap[country].Plan, models.JourneyListItem{
			ID:          j.ID,
			Slug:        v.Slug,
			Title:       title,
			Destination: fmt.Sprintf("%s, %s", v.Destination, v.Country),
			CoverURL:    v.CoverURL,
			Status:      v.Status,
		})
	}

	result := make([]models.JourneyByCountry, 0, len(order))
	for _, c := range order {
		result = append(result, *groupedMap[c])
	}

	return utils.Success(c, fiber.StatusOK, result)
}

func (h *JourneyHandler) GetBySlug(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		return utils.Error(c, fiber.StatusBadRequest, "invalid journey slug")
	}

	view, err := h.privacy.View(c.Context(), middleware.CurrentUserID(c), slug)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, view)
}

func (h *JourneyHandler) Update(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		return utils.Error(c, fiber.StatusBadRequest, "invalid journey slug")
	}

	var journey models.Journey
	if err := c.BodyParser(&journey); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if journey.Title == "" || journey.Destination == "" {
		return utils.Error(c, fiber.StatusBadRequest, "title and destination are required")
	}

	if err := h.svc.Update(c.Context(), middleware.CurrentUserID(c), slug, &journey); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, nil)
}

func (h *JourneyHandler) Delete(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		return utils.Error(c, fiber.StatusBadRequest, "invalid journey slug")
	}

	if err := h.svc.Delete(c.Context(), middleware.CurrentUserID(c), slug); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}
