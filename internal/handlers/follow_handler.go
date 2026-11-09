package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type FollowHandler struct {
	svc *services.FollowService
}

func NewFollowHandler(svc *services.FollowService) *FollowHandler {
	return &FollowHandler{svc: svc}
}

func (h *FollowHandler) Follow(c *fiber.Ctx) error {
	st, err := h.svc.Follow(c.Context(), middleware.CurrentUserID(c), c.Params("username"))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, st)
}

func (h *FollowHandler) Unfollow(c *fiber.Ctx) error {
	st, err := h.svc.Unfollow(c.Context(), middleware.CurrentUserID(c), c.Params("username"))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, st)
}
