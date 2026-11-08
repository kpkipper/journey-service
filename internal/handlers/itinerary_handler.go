package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type ItineraryHandler struct {
	svc *services.ItineraryService
}

func NewItineraryHandler(svc *services.ItineraryService) *ItineraryHandler {
	return &ItineraryHandler{svc: svc}
}

func (h *ItineraryHandler) PatchJourney(c *fiber.Ctx) error {
	var in services.JourneyPatch
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	j, err := h.svc.PatchJourney(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), in)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, j)
}

func (h *ItineraryHandler) AddDay(c *fiber.Ctx) error {
	var in services.DayInput
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	d, err := h.svc.AddDay(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), in)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, d)
}

func (h *ItineraryHandler) UpdateDay(c *fiber.Ctx) error {
	var in services.DayInput
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	d, err := h.svc.UpdateDay(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("dayId"), in)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, d)
}

func (h *ItineraryHandler) DeleteDay(c *fiber.Ctx) error {
	if err := h.svc.DeleteDay(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("dayId")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}

func (h *ItineraryHandler) AddPlan(c *fiber.Ctx) error {
	var in services.PlanInput
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	p, err := h.svc.AddPlan(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("dayId"), in)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, p)
}

func (h *ItineraryHandler) UpdatePlan(c *fiber.Ctx) error {
	var in services.PlanInput
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	p, err := h.svc.UpdatePlan(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("planId"), in)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, p)
}

func (h *ItineraryHandler) DeletePlan(c *fiber.Ctx) error {
	if err := h.svc.DeletePlan(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("planId")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}

func (h *ItineraryHandler) ReorderPlans(c *fiber.Ctx) error {
	var in struct {
		PlanIDs []string `json:"plan_ids"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.svc.ReorderPlans(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("dayId"), in.PlanIDs); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}
