package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/models"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type UserHandler struct {
	svc *services.UserService
}

func NewUserHandler(svc *services.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Me(c *fiber.Ctx) error {
	user, err := h.svc.GetByID(c.Context(), middleware.CurrentUserID(c))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, user)
}

func (h *UserHandler) UpdateMe(c *fiber.Ctx) error {
	var in models.ProfileUpdate
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	user, err := h.svc.UpdateProfile(c.Context(), middleware.CurrentUserID(c), in)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, user)
}

func (h *UserHandler) PublicProfile(c *fiber.Ctx) error {
	profile, err := h.svc.PublicProfileFor(c.Context(), middleware.CurrentUserID(c), c.Params("username"))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, profile)
}
