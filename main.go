package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/config"
	"github.com/kpkipper/journey-service/database"
	"github.com/kpkipper/journey-service/internal/admin"
	appPkg "github.com/kpkipper/journey-service/internal/app"
	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/mailer"
	"github.com/kpkipper/journey-service/internal/media"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/internal/storage"
	"github.com/kpkipper/journey-service/pkg/logger"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Get()
	logger.Init(cfg.App.ENV != "production")
	log := logger.Get()

	isAssignLegacy := len(os.Args) > 1 && os.Args[1] == "assign-legacy-owner"
	if isAssignLegacy && len(os.Args) < 3 {
		log.Fatal().Msg("usage: server assign-legacy-owner <username>")
	}
	isReports := len(os.Args) > 1 && os.Args[1] == "reports"
	isResolveReport := len(os.Args) > 1 && os.Args[1] == "resolve-report"
	if isResolveReport && len(os.Args) < 3 {
		log.Fatal().Msg("usage: server resolve-report <id>")
	}
	isDeleteMoment := len(os.Args) > 1 && os.Args[1] == "delete-moment"
	if isDeleteMoment && len(os.Args) < 3 {
		log.Fatal().Msg("usage: server delete-moment <id>")
	}

	log.Info().Str("host", cfg.DBDSN.Host).Msg("connecting to database")
	db, err := database.NewConnection(cfg.DBDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	log.Info().Msg("database connected")

	if err := database.Migrate(db); err != nil {
		log.Fatal().Err(err).Msg("failed to migrate database")
	}
	log.Info().Msg("database migrated")

	// CLI command: runs before the mailer/web-URL checks so it works on a box without SMTP.
	if isAssignLegacy {
		n, err := admin.AssignLegacyOwner(context.Background(), db, os.Args[2])
		if err != nil {
			log.Fatal().Err(err).Msg("assign-legacy-owner failed")
		}
		log.Info().Int64("journeys", n).Str("username", os.Args[2]).Msg("legacy journeys assigned")
		os.Exit(0)
	}
	if isReports || isResolveReport {
		reports := services.NewReportService(services.ReportDeps{Reports: repository.NewReportRepository(db)})
		if isReports {
			err = admin.PrintOpenReports(context.Background(), os.Stdout, reports)
		} else {
			err = admin.ResolveReport(context.Background(), reports, os.Args[2])
		}
		if err != nil {
			log.Fatal().Err(err).Msg(os.Args[1] + " failed")
		}
		os.Exit(0)
	}

	var store storage.Store
	var localStore *storage.LocalStore
	driver := strings.ToLower(strings.TrimSpace(cfg.Storage.Driver))
	if driver == "" && allowLocalStorage(cfg.App.ENV) {
		driver = "local"
	}
	switch driver {
	case "local":
		if !allowLocalStorage(cfg.App.ENV) {
			log.Fatal().Str("app_env", cfg.App.ENV).Msg("STORAGE_DRIVER=local is only allowed when APP_ENV is development or test; use firebase")
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			log.Fatal().Err(err).Msg("failed to generate local storage secret")
		}
		base := strings.TrimRight(cfg.Storage.PublicAPIURL, "/")
		if base == "" {
			base = fmt.Sprintf("http://localhost:%d/api/v1", cfg.App.Port)
		}
		localStore, err = storage.NewLocalStore(cfg.Storage.LocalDir, base+"/media/local", secret)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to set up local storage")
		}
		store = localStore
		log.Warn().Str("dir", cfg.Storage.LocalDir).Msg("using local media storage (development only)")
	case "firebase":
		if cfg.Storage.FirebaseBucket == "" || strings.TrimSpace(cfg.Storage.FirebaseCredential) == "" {
			log.Fatal().Msg("FIREBASE_STORAGE_BUCKET and FIREBASE_SERVICE_ACCOUNT_JSON must be set when STORAGE_DRIVER=firebase")
		}
		store, err = storage.NewFirebaseStore(context.Background(), cfg.Storage.FirebaseBucket, []byte(cfg.Storage.FirebaseCredential))
		if err != nil {
			log.Fatal().Err(err).Msg("failed to set up firebase storage")
		}
	default:
		log.Fatal().Str("storage_driver", cfg.Storage.Driver).Str("app_env", cfg.App.ENV).Msg("STORAGE_DRIVER must be firebase (or local when APP_ENV is development or test)")
	}

	// delete-moment needs storage but not SMTP, so it runs once storage is built.
	if isDeleteMoment {
		svc := services.NewMomentService(services.MomentDeps{
			Access:  services.NewAccessService(repository.NewJourneyRepository(db), repository.NewMemberRepository(db)),
			Moments: repository.NewMomentRepository(db), Store: store,
		})
		if err := admin.DeleteMoment(context.Background(), svc, os.Args[2]); err != nil {
			log.Fatal().Err(err).Msg("delete-moment failed")
		}
		log.Info().Str("moment", os.Args[2]).Msg("moment deleted")
		os.Exit(0)
	}

	if cfg.Google.ClientID == "" {
		log.Warn().Msg("GOOGLE_CLIENT_ID is empty; Google login will be rejected")
	}

	var mail mailer.Mailer
	if cfg.SMTP.Host == "" {
		if !allowLogMailer(cfg.App.ENV) {
			log.Fatal().Str("app_env", cfg.App.ENV).Msg("SMTP_HOST must be set unless APP_ENV is development or test (the log mailer prints login links)")
		}
		if !validWebURL(cfg.App.WebURL) {
			log.Warn().Str("app_web_url", cfg.App.WebURL).Msg("APP_WEB_URL is not an absolute http(s) URL; login links will be broken")
		}
		mail = mailer.NewLogMailer()
	} else {
		if !validWebURL(cfg.App.WebURL) {
			log.Fatal().Str("app_web_url", cfg.App.WebURL).Msg("APP_WEB_URL must be an absolute http or https URL when SMTP is used")
		}
		mail = mailer.NewSMTPMailer(cfg.SMTP)
	}

	// A true nil interface (not a typed-nil *FFmpeg) when ffmpeg is missing.
	var video media.VideoProcessor
	if f := media.NewFFmpeg(); f.Available() {
		video = f
	} else {
		log.Warn().Msg("ffmpeg/ffprobe not found in PATH; video moments cannot be completed")
	}

	app := appPkg.New(appPkg.Deps{
		DB:         db,
		Google:     auth.NewGoogleVerifier(cfg.Google.ClientID),
		Mailer:     mail,
		WebURL:     cfg.App.WebURL,
		Store:      store,
		LocalStore: localStore,
		Video:      video,
	})
	startMomentCleanup(db, store, video)

	setupGracefulShutdown(app)

	addr := fmt.Sprintf(":%d", cfg.App.Port)
	log.Info().Str("addr", addr).Msg("server starting")
	if err := app.Listen(addr); err != nil {
		log.Error().Err(err).Msg("server error")
	}
}

// startMomentCleanup removes uploads left pending for over an hour, every 30 minutes.
func startMomentCleanup(db *gorm.DB, store storage.Store, video media.VideoProcessor) {
	svc := services.NewMomentService(services.MomentDeps{
		Access:  services.NewAccessService(repository.NewJourneyRepository(db), repository.NewMemberRepository(db)),
		Moments: repository.NewMomentRepository(db), Store: store, Video: video,
	})
	go func() {
		t := time.NewTicker(30 * time.Minute)
		defer t.Stop()
		for range t.C {
			n, err := svc.CleanupPending(context.Background())
			if err != nil {
				logger.Get().Error().Err(err).Msg("moment cleanup failed")
			} else if n > 0 {
				logger.Get().Info().Int("removed", n).Msg("moment cleanup")
			}
		}
	}()
}

// allowLocalStorage reports whether the dev-only local media store may be used.
func allowLocalStorage(env string) bool { return allowLogMailer(env) }

// allowLogMailer reports whether the dev-only log mailer (prints raw login links) may be used.
func allowLogMailer(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "development", "test":
		return true
	}
	return false
}

// validWebURL reports whether s is an absolute http or https URL with a host.
func validWebURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func setupGracefulShutdown(app *fiber.App) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log := logger.Get()
		log.Info().Msg("shutting down server")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := app.ShutdownWithContext(ctx); err != nil {
			log.Error().Err(err).Msg("server forced to shutdown")
		}

		log.Info().Msg("server exited")
		os.Exit(0)
	}()
}
