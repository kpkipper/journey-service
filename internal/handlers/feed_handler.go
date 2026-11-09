package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type FeedHandler struct {
	svc *services.FeedService
}

func NewFeedHandler(svc *services.FeedService) *FeedHandler { return &FeedHandler{svc: svc} }

func (h *FeedHandler) Feed(c *fiber.Ctx) error {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return utils.Error(c, fiber.StatusBadRequest, "limit must be an integer")
		}
		limit = n
	}
	page, err := h.svc.Feed(c.Context(), middleware.CurrentUserID(c), c.Query("before"), limit)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, page)
}

func (h *FeedHandler) Now(c *fiber.Ctx) error {
	list, err := h.svc.Now(c.Context(), middleware.CurrentUserID(c))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"travelers": list})
}
