package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type JourneyHandler struct {
	svc *services.JourneyService
}

func NewJourneyHandler(svc *services.JourneyService) *JourneyHandler {
	return &JourneyHandler{svc: svc}
}

func (h *JourneyHandler) Create(c *fiber.Ctx) error {
	var journey models.Journey
	if err := c.BodyParser(&journey); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if journey.Title == "" || journey.Destination == "" {
		return utils.Error(c, fiber.StatusBadRequest, "title and destination are required")
	}

	if err := h.svc.Create(c.Context(), &journey); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusCreated, journey)
}

func (h *JourneyHandler) List(c *fiber.Ctx) error {
	journeys, err := h.svc.List(c.Context())
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
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
			Title:       j.Title,
			Destination: j.Destination,
		})
	}

	result := make([]models.JourneyByCountry, 0, len(order))
	for _, c := range order {
		result = append(result, *groupedMap[c])
	}

	return utils.Success(c, fiber.StatusOK, result)
}

func (h *JourneyHandler) GetByID(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid journey id")
	}

	journey, err := h.svc.GetByID(c.Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "journey not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, journey)
}

func (h *JourneyHandler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid journey id")
	}

	var journey models.Journey
	if err := c.BodyParser(&journey); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if journey.Title == "" || journey.Destination == "" {
		return utils.Error(c, fiber.StatusBadRequest, "title and destination are required")
	}

	updated, err := h.svc.Update(c.Context(), id, &journey)
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "journey not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, updated)
}

func (h *JourneyHandler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid journey id")
	}

	if err := h.svc.Delete(c.Context(), id); err != nil {
		if errors.Is(err, services.ErrNotFound) {
			return utils.Error(c, fiber.StatusNotFound, "journey not found")
		}
		return utils.Error(c, fiber.StatusInternalServerError, err.Error())
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}
