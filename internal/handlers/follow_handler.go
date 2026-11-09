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

func (h *FollowHandler) Requests(c *fiber.Ctx) error {
	uid := middleware.CurrentUserID(c)
	items, err := h.svc.Requests(c.Context(), uid)
	if err != nil {
		return respondError(c, err)
	}
	n, err := h.svc.RequestCount(c.Context(), uid)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"items": items, "count": n})
}

func (h *FollowHandler) Accept(c *fiber.Ctx) error {
	if err := h.svc.Accept(c.Context(), middleware.CurrentUserID(c), c.Params("username")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"accepted": true})
}

func (h *FollowHandler) Decline(c *fiber.Ctx) error {
	if err := h.svc.Decline(c.Context(), middleware.CurrentUserID(c), c.Params("username")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"declined": true})
}

func (h *FollowHandler) Followers(c *fiber.Ctx) error {
	items, err := h.svc.Followers(c.Context(), middleware.CurrentUserID(c))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"items": items})
}

func (h *FollowHandler) RemoveFollower(c *fiber.Ctx) error {
	if err := h.svc.RemoveFollower(c.Context(), middleware.CurrentUserID(c), c.Params("username")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"removed": true})
}
