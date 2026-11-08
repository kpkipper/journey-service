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
	username := c.Params("username")
	if err := h.svc.Follow(c.Context(), middleware.CurrentUserID(c), username); err != nil {
		return respondError(c, err)
	}
	return h.respond(c, username, true)
}

func (h *FollowHandler) Unfollow(c *fiber.Ctx) error {
	username := c.Params("username")
	if err := h.svc.Unfollow(c.Context(), middleware.CurrentUserID(c), username); err != nil {
		return respondError(c, err)
	}
	return h.respond(c, username, false)
}

// respond returns the new follow state and the followed user's follower count.
func (h *FollowHandler) respond(c *fiber.Ctx, username string, following bool) error {
	followers, _, err := h.svc.Counts(c.Context(), username)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"is_following": following, "followers": followers})
}
