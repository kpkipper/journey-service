package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type BlockHandler struct {
	svc *services.BlockService
}

func NewBlockHandler(svc *services.BlockService) *BlockHandler { return &BlockHandler{svc: svc} }

func (h *BlockHandler) Block(c *fiber.Ctx) error {
	if err := h.svc.Block(c.Context(), middleware.CurrentUserID(c), c.Params("username")); err != nil {
		return respondError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *BlockHandler) Unblock(c *fiber.Ctx) error {
	if err := h.svc.Unblock(c.Context(), middleware.CurrentUserID(c), c.Params("username")); err != nil {
		return respondError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *BlockHandler) List(c *fiber.Ctx) error {
	users, err := h.svc.List(c.Context(), middleware.CurrentUserID(c))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"users": users})
}
