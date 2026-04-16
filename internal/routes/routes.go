package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/handlers"
)

func Register(app *fiber.App, h *handlers.JourneyHandler) {
	v1 := app.Group("/api/v1")

	journeys := v1.Group("/journeys")
	journeys.Post("/", h.Create)
	journeys.Get("/", h.List)
	journeys.Get("/:id", h.GetByID)
	journeys.Put("/:id", h.Update)
	journeys.Delete("/", h.DeleteAll)
	journeys.Delete("/:id", h.Delete)
}
