package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/handlers"
)

// Register wires all routes. requireAuth is applied per route (not via group.Use)
// so unmatched routes still return 404/405.
func Register(app *fiber.App, j *handlers.JourneyHandler, a *handlers.AuthHandler, u *handlers.UserHandler, m *handlers.MemberHandler, it *handlers.ItineraryHandler, f *handlers.FollowHandler, p *handlers.ProfileHandler, mo *handlers.MomentHandler, bl *handlers.BlockHandler, rp *handlers.ReportHandler, fd *handlers.FeedHandler, requireAuth, optionalAuth fiber.Handler) {
	v1 := app.Group("/api/v1")

	authG := v1.Group("/auth")
	authG.Post("/google", a.Google)
	authG.Post("/email/start", a.EmailStart)
	authG.Post("/email/verify", a.EmailVerify)
	authG.Post("/logout", requireAuth, a.Logout)

	v1.Get("/me", requireAuth, u.Me)
	v1.Patch("/me", requireAuth, u.UpdateMe)
	v1.Get("/users/:username", optionalAuth, u.PublicProfile)
	v1.Get("/users/:username/profile", optionalAuth, p.Get)
	v1.Post("/users/:username/follow", requireAuth, f.Follow)
	v1.Delete("/users/:username/follow", requireAuth, f.Unfollow)
	v1.Post("/users/:username/block", requireAuth, bl.Block)
	v1.Delete("/users/:username/block", requireAuth, bl.Unblock)
	v1.Post("/reports", requireAuth, rp.Create)
	v1.Get("/feed", requireAuth, fd.Feed)
	v1.Get("/feed/now", requireAuth, fd.Now)

	// Static /me/* and /invites/* paths; none collides with a param route.
	v1.Get("/me/invites", requireAuth, m.MyInvites)
	v1.Post("/me/invites/:id/accept", requireAuth, m.Accept)
	v1.Post("/me/invites/:id/decline", requireAuth, m.Decline)
	v1.Get("/me/journeys", requireAuth, m.MyJourneys)
	v1.Get("/me/moments", requireAuth, mo.Mine)
	v1.Get("/me/blocks", requireAuth, bl.List)
	v1.Post("/invites/join", requireAuth, m.JoinByLink)

	v1.Patch("/moments/:id", requireAuth, mo.UpdateCaption)
	v1.Delete("/moments/:id", requireAuth, mo.Delete)

	journeys := v1.Group("/journeys")
	journeys.Post("/create", requireAuth, j.Create)
	journeys.Get("/", optionalAuth, j.List)
	journeys.Get("/:slug", optionalAuth, j.GetBySlug)
	journeys.Put("/:slug/update", requireAuth, j.Update)
	journeys.Delete("/:slug", requireAuth, j.Delete)
	journeys.Patch("/:slug", requireAuth, it.PatchJourney)

	journeys.Post("/:slug/days", requireAuth, it.AddDay)
	journeys.Patch("/:slug/days/:dayId", requireAuth, it.UpdateDay)
	journeys.Delete("/:slug/days/:dayId", requireAuth, it.DeleteDay)
	journeys.Post("/:slug/days/:dayId/plans", requireAuth, it.AddPlan)
	journeys.Put("/:slug/days/:dayId/plans/order", requireAuth, it.ReorderPlans)
	journeys.Patch("/:slug/plans/:planId", requireAuth, it.UpdatePlan)
	journeys.Delete("/:slug/plans/:planId", requireAuth, it.DeletePlan)

	// Static "uploads" before the :id param.
	journeys.Post("/:slug/moments/uploads", requireAuth, mo.StartUpload)
	journeys.Post("/:slug/moments/:id/complete", requireAuth, mo.Complete)
	journeys.Get("/:slug/moments", optionalAuth, mo.List)

	journeys.Get("/:slug/members", requireAuth, m.List)
	journeys.Post("/:slug/members", requireAuth, m.Invite)
	journeys.Patch("/:slug/members/:username", requireAuth, m.ChangeRole)
	journeys.Delete("/:slug/members/:username", requireAuth, m.Remove)
	journeys.Post("/:slug/leave", requireAuth, m.Leave)
	journeys.Get("/:slug/invite-links", requireAuth, m.ListInviteLinks)
	journeys.Post("/:slug/invite-links", requireAuth, m.CreateInviteLink)
	journeys.Delete("/:slug/invite-links/:id", requireAuth, m.RevokeInviteLink)
}
