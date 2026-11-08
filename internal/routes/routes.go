package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/handlers"
)

// Register wires all routes. requireAuth is applied per route (not via group.Use)
// so unmatched routes still return 404/405.
func Register(app *fiber.App, j *handlers.JourneyHandler, a *handlers.AuthHandler, u *handlers.UserHandler, requireAuth fiber.Handler) {
	v1 := app.Group("/api/v1")

	authG := v1.Group("/auth")
	authG.Post("/google", a.Google)
	authG.Post("/email/start", a.EmailStart)
	authG.Post("/email/verify", a.EmailVerify)
	authG.Post("/logout", requireAuth, a.Logout)

	v1.Get("/me", requireAuth, u.Me)
	v1.Patch("/me", requireAuth, u.UpdateMe)
	v1.Get("/users/:username", u.PublicProfile)

	journeys := v1.Group("/journeys")
	journeys.Post("/create", requireAuth, j.Create)
	journeys.Get("/", j.List)
	journeys.Get("/:slug", j.GetBySlug)
	journeys.Put("/:slug/update", requireAuth, j.Update)
	journeys.Delete("/:slug", requireAuth, j.Delete)
}
