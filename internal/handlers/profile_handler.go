package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type ProfileHandler struct {
	svc *services.ProfileService
}

func NewProfileHandler(svc *services.ProfileService) *ProfileHandler {
	return &ProfileHandler{svc: svc}
}

// Get returns the profile page as the (optional) viewer may see it.
func (h *ProfileHandler) Get(c *fiber.Ctx) error {
	p, err := h.svc.Get(c.Context(), middleware.CurrentUserID(c), c.Params("username"))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, p)
}
