package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type ReportHandler struct {
	svc *services.ReportService
}

func NewReportHandler(svc *services.ReportService) *ReportHandler { return &ReportHandler{svc: svc} }

// Create files a report: 201 when new, 200 when an open duplicate exists.
// The response never includes the target id.
func (h *ReportHandler) Create(c *fiber.Ctx) error {
	var in services.ReportInput
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	r, created, err := h.svc.Create(c.Context(), middleware.CurrentUserID(c), in)
	if err != nil {
		return respondError(c, err)
	}
	status := fiber.StatusOK
	if created {
		status = fiber.StatusCreated
	}
	return utils.Success(c, status, fiber.Map{
		"id": r.ID, "target_type": r.TargetType, "reason": r.Reason, "status": r.Status, "created_at": r.CreatedAt,
	})
}
