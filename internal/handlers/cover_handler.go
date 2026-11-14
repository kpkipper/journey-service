package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type CoverHandler struct {
	svc *services.CoverService
}

func NewCoverHandler(svc *services.CoverService) *CoverHandler {
	return &CoverHandler{svc: svc}
}

func (h *CoverHandler) StartUpload(c *fiber.Ctx) error {
	var in services.CoverUploadInput
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	out, err := h.svc.StartUpload(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), in)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, out)
}

func (h *CoverHandler) Complete(c *fiber.Ctx) error {
	var in struct {
		UploadID string `json:"upload_id"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	out, err := h.svc.Complete(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), in.UploadID)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, out)
}

func (h *CoverHandler) Remove(c *fiber.Ctx) error {
	if err := h.svc.Remove(c.Context(), middleware.CurrentUserID(c), c.Params("slug")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}
