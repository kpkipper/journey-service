package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type MomentHandler struct {
	svc *services.MomentService
}

func NewMomentHandler(svc *services.MomentService) *MomentHandler {
	return &MomentHandler{svc: svc}
}

func (h *MomentHandler) StartUpload(c *fiber.Ctx) error {
	var in services.StartUploadInput
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	out, err := h.svc.StartUpload(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), in)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, out)
}

func (h *MomentHandler) Complete(c *fiber.Ctx) error {
	out, err := h.svc.Complete(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("id"))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, out)
}

func (h *MomentHandler) List(c *fiber.Ctx) error {
	out, err := h.svc.List(c.Context(), middleware.CurrentUserID(c), c.Params("slug"))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"moments": out})
}

func (h *MomentHandler) UpdateCaption(c *fiber.Ctx) error {
	var in struct {
		Caption string `json:"caption"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	out, err := h.svc.UpdateCaption(c.Context(), middleware.CurrentUserID(c), c.Params("id"), in.Caption)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, out)
}

func (h *MomentHandler) Delete(c *fiber.Ctx) error {
	if err := h.svc.Delete(c.Context(), middleware.CurrentUserID(c), c.Params("id")); err != nil {
		return respondError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *MomentHandler) Mine(c *fiber.Ctx) error {
	out, err := h.svc.ListMine(c.Context(), middleware.CurrentUserID(c))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"moments": out})
}
