package app

import (
	"context"
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
	"github.com/kpkipper/journey-service/pkg/logger"
	"gorm.io/gorm"
)

type Deps struct {
	DB     *gorm.DB
	Google auth.GoogleVerifier
	// GoogleEnabled says GOOGLE_CLIENT_ID is set (reported by GET /auth/methods).
	GoogleEnabled bool
	Mailer        mailer.Mailer // nil = email login is off
	WebURL        string
	Now           func() time.Time // nil = time.Now

	Store      storage.Store        // nil = in-memory store (tests)
	LocalStore *storage.LocalStore  // non-nil mounts /media/local/* and raises BodyLimit
	Video      media.VideoProcessor // nil interface = video uploads unavailable
	TempDir    string               // "" = os.TempDir()

	ClientIPHeader string // header carrying the caller's IP; "" = DefaultClientIPHeader
}

// New builds the repositories, services and handlers and returns the configured Fiber app.
func New(d Deps) *fiber.App {
	journeyRepo := repository.NewJourneyRepository(d.DB)
	userRepo := repository.NewUserRepository(d.DB)
	authRepo := repository.NewAuthRepository(d.DB)

	memberRepo := repository.NewMemberRepository(d.DB)
	followRepo := repository.NewFollowRepository(d.DB)
	blockRepo := repository.NewBlockRepository(d.DB)
	accessSvc := services.NewAccessService(journeyRepo, memberRepo).WithFollows(followRepo).WithBlocks(blockRepo)
	store := d.Store
	if store == nil {
		store = storage.NewMemoryStore()
	}
	coverSvc := services.NewCoverService(services.CoverDeps{Access: accessSvc, Journeys: journeyRepo, Store: store, Now: d.Now})
	privacySvc := services.NewPrivacyService(services.PrivacyDeps{
		Access: accessSvc, Follows: followRepo, Members: memberRepo, Users: userRepo, Covers: coverSvc, Now: d.Now,
	})
	itineraryRepo := repository.NewItineraryRepository(d.DB)
	momentRepo := repository.NewMomentRepository(d.DB)
	tempDir := d.TempDir
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	momentSvc := services.NewMomentService(services.MomentDeps{
		Access: accessSvc, Privacy: privacySvc, Moments: momentRepo, Itinerary: itineraryRepo,
		Users: userRepo, Blocks: blockRepo, Store: store, Video: d.Video, TempDir: tempDir, Now: d.Now,
	})
	journeySvc := services.NewJourneyService(journeyRepo, accessSvc).WithMoments(momentRepo, func(ctx context.Context, journeyID string) error {
		// Best effort, like the moments: a leftover cover file is only storage.
		if err := coverSvc.DeleteForJourney(ctx, journeyID); err != nil {
			logger.Get().Error().Err(err).Str("journey", journeyID).Msg("delete trip cover")
		}
		return momentSvc.DeleteForJourney(ctx, journeyID)
	})
	itinerarySvc := services.NewItineraryService(accessSvc, itineraryRepo, journeyRepo, momentRepo)
	memberSvc := services.NewMemberService(services.MemberDeps{
		Access:   accessSvc,
		Journeys: journeyRepo,
		Members:  memberRepo,
		Users:    userRepo,
		Blocks:   blockRepo,
		Covers:   coverSvc,
		WebURL:   strings.TrimRight(d.WebURL, "/"),
		Now:      d.Now,
	})
	userSvc := services.NewUserService(userRepo).WithBlocks(blockRepo).WithFollows(followRepo)
	followSvc := services.NewFollowService(userRepo, followRepo).WithBlocks(blockRepo)
	profileSvc := services.NewProfileService(services.ProfileDeps{
		Users: userRepo, Journeys: journeyRepo, Members: memberRepo, Follows: followRepo, Blocks: blockRepo, Privacy: privacySvc,
	})
	blockSvc := services.NewBlockService(userRepo, blockRepo)
	reportSvc := services.NewReportService(services.ReportDeps{
		Reports: repository.NewReportRepository(d.DB), Users: userRepo, Privacy: privacySvc,
		Moments: momentSvc, Blocks: blockRepo, Now: d.Now,
	})
	feedSvc := services.NewFeedService(services.FeedDeps{
		Journeys: journeyRepo, Members: memberRepo, Follows: followRepo, Blocks: blockRepo, Users: userRepo,
		Moments: momentRepo, Privacy: privacySvc, MomentSvc: momentSvc, Clock: d.Now,
	})
	authSvc := services.NewAuthService(services.AuthDeps{
		Users:         userRepo,
		Auth:          authRepo,
		UserSvc:       userSvc,
		Mailer:        d.Mailer,
		Google:        d.Google,
		GoogleEnabled: d.GoogleEnabled,
		WebURL:        strings.TrimRight(d.WebURL, "/"),
		Now:           d.Now,
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
	ipHeader := d.ClientIPHeader
	if ipHeader == "" {
		ipHeader = DefaultClientIPHeader
	}
	// Runs before the route of the same path (it calls c.Next()).
	app.Post("/api/v1/auth/email/start", loginEmailLimiter(ipHeader))
	routes.Register(app,
		handlers.NewJourneyHandler(journeySvc, privacySvc),
		handlers.NewAuthHandler(authSvc),
		handlers.NewUserHandler(userSvc),
		handlers.NewMemberHandler(memberSvc),
		handlers.NewItineraryHandler(itinerarySvc),
		handlers.NewFollowHandler(followSvc),
		handlers.NewProfileHandler(profileSvc),
		handlers.NewMomentHandler(momentSvc),
		handlers.NewBlockHandler(blockSvc),
		handlers.NewReportHandler(reportSvc),
		handlers.NewFeedHandler(feedSvc),
		handlers.NewCoverHandler(coverSvc),
		middleware.RequireAuth(authSvc),
		middleware.OptionalAuth(authSvc),
	)
	if d.LocalStore != nil {
		d.LocalStore.Register(app.Group("/api/v1"))
	}
	return app
}
