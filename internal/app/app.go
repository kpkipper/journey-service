package app

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/handlers"
	"github.com/kpkipper/journey-service/internal/mailer"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/routes"
	"github.com/kpkipper/journey-service/internal/services"
	"gorm.io/gorm"
)

type Deps struct {
	DB     *gorm.DB
	Google auth.GoogleVerifier
	Mailer mailer.Mailer
	WebURL string
	Now    func() time.Time // nil = time.Now
}

// New builds the repositories, services and handlers and returns the configured Fiber app.
func New(d Deps) *fiber.App {
	journeyRepo := repository.NewJourneyRepository(d.DB)
	userRepo := repository.NewUserRepository(d.DB)
	authRepo := repository.NewAuthRepository(d.DB)

	journeySvc := services.NewJourneyService(journeyRepo)
	userSvc := services.NewUserService(userRepo)
	authSvc := services.NewAuthService(services.AuthDeps{
		Users:   userRepo,
		Auth:    authRepo,
		UserSvc: userSvc,
		Mailer:  d.Mailer,
		Google:  d.Google,
		WebURL:  strings.TrimRight(d.WebURL, "/"),
		Now:     d.Now,
	})

	app := fiber.New(fiber.Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})
	middleware.Register(app)
	routes.Register(app,
		handlers.NewJourneyHandler(journeySvc),
		handlers.NewAuthHandler(authSvc),
		handlers.NewUserHandler(userSvc),
		middleware.RequireAuth(authSvc),
	)
	return app
}
