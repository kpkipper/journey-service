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
	members *services.MemberService
}

func NewJourneyHandler(svc *services.JourneyService, members *services.MemberService) *JourneyHandler {
	return &JourneyHandler{svc: svc, members: members}
}

// journeyDetail is the public journey with its owner and joined members.
type journeyDetail struct {
	*models.Journey
	Members []models.MemberView `json:"members"`
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

func (h *JourneyHandler) List(c *fiber.Ctx) error {
	journeys, err := h.svc.List(c.Context())
	if err != nil {
		return respondError(c, err)
	}

	groupedMap := map[string]*models.JourneyByCountry{}
	order := []string{}
	for _, j := range journeys {
		country := j.Country
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
		groupedMap[country].Plan = append(groupedMap[country].Plan, models.JourneyListItem{
			ID:          j.ID,
			Slug:        j.Slug,
			Title:       fmt.Sprintf("%s %d", j.Destination, j.DepartureDate.Year()),
			Destination: fmt.Sprintf("%s, %s", j.Destination, j.Country),
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

	journey, err := h.svc.GetBySlug(c.Context(), slug)
	if err != nil {
		return respondError(c, err)
	}
	members, err := h.members.PublicMembers(c.Context(), slug)
	if err != nil {
		return respondError(c, err)
	}
	if members == nil {
		members = []models.MemberView{}
	}
	return utils.Success(c, fiber.StatusOK, journeyDetail{Journey: journey, Members: members})
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
