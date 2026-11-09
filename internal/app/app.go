package app

import (
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/handlers"
	"github.com/kpkipper/journey-service/internal/mailer"
	"github.com/kpkipper/journey-service/internal/media"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/routes"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/internal/storage"
	"gorm.io/gorm"
)

type Deps struct {
	DB     *gorm.DB
	Google auth.GoogleVerifier
	Mailer mailer.Mailer
	WebURL string
	Now    func() time.Time // nil = time.Now

	Store      storage.Store        // nil = in-memory store (tests)
	LocalStore *storage.LocalStore  // non-nil mounts /media/local/* and raises BodyLimit
	Video      media.VideoProcessor // nil interface = video uploads unavailable
	TempDir    string               // "" = os.TempDir()
}

// New builds the repositories, services and handlers and returns the configured Fiber app.
func New(d Deps) *fiber.App {
	journeyRepo := repository.NewJourneyRepository(d.DB)
	userRepo := repository.NewUserRepository(d.DB)
	authRepo := repository.NewAuthRepository(d.DB)

	memberRepo := repository.NewMemberRepository(d.DB)
	followRepo := repository.NewFollowRepository(d.DB)
	accessSvc := services.NewAccessService(journeyRepo, memberRepo).WithFollows(followRepo)
	privacySvc := services.NewPrivacyService(services.PrivacyDeps{
		Access: accessSvc, Follows: followRepo, Members: memberRepo, Users: userRepo, Now: d.Now,
	})
	itineraryRepo := repository.NewItineraryRepository(d.DB)
	momentRepo := repository.NewMomentRepository(d.DB)
	store := d.Store
	if store == nil {
		store = storage.NewMemoryStore()
	}
	tempDir := d.TempDir
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	momentSvc := services.NewMomentService(services.MomentDeps{
		Access: accessSvc, Privacy: privacySvc, Moments: momentRepo, Itinerary: itineraryRepo,
		Users: userRepo, Store: store, Video: d.Video, TempDir: tempDir, Now: d.Now,
	})
	journeySvc := services.NewJourneyService(journeyRepo, accessSvc).WithMoments(momentRepo, momentSvc.DeleteForJourney)
	itinerarySvc := services.NewItineraryService(accessSvc, itineraryRepo, journeyRepo, momentRepo)
	memberSvc := services.NewMemberService(services.MemberDeps{
		Access:   accessSvc,
		Journeys: journeyRepo,
		Members:  memberRepo,
		Users:    userRepo,
		WebURL:   strings.TrimRight(d.WebURL, "/"),
		Now:      d.Now,
	})
	userSvc := services.NewUserService(userRepo)
	followSvc := services.NewFollowService(userRepo, followRepo)
	profileSvc := services.NewProfileService(services.ProfileDeps{
		Users: userRepo, Journeys: journeyRepo, Members: memberRepo, Follows: followRepo, Privacy: privacySvc,
	})
	authSvc := services.NewAuthService(services.AuthDeps{
		Users:   userRepo,
		Auth:    authRepo,
		UserSvc: userSvc,
		Mailer:  d.Mailer,
		Google:  d.Google,
		WebURL:  strings.TrimRight(d.WebURL, "/"),
		Now:     d.Now,
	})

	cfg := fiber.Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	if d.LocalStore != nil {
		// Local PUTs carry the raw file (up to 100 MB video); development only.
		cfg.BodyLimit = 105 * 1024 * 1024
	}
	app := fiber.New(cfg)
	middleware.Register(app)
	routes.Register(app,
		handlers.NewJourneyHandler(journeySvc, privacySvc),
		handlers.NewAuthHandler(authSvc),
		handlers.NewUserHandler(userSvc),
		handlers.NewMemberHandler(memberSvc),
		handlers.NewItineraryHandler(itinerarySvc),
		handlers.NewFollowHandler(followSvc),
		handlers.NewProfileHandler(profileSvc),
		handlers.NewMomentHandler(momentSvc),
		middleware.RequireAuth(authSvc),
		middleware.OptionalAuth(authSvc),
	)
	if d.LocalStore != nil {
		d.LocalStore.Register(app.Group("/api/v1"))
	}
	return app
}
