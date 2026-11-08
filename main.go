package main

import (
	"context"
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
	"github.com/kpkipper/journey-service/pkg/logger"
)

func main() {
	cfg := config.Get()
	logger.Init(cfg.App.ENV != "production")
	log := logger.Get()

	isAssignLegacy := len(os.Args) > 1 && os.Args[1] == "assign-legacy-owner"
	if isAssignLegacy && len(os.Args) < 3 {
		log.Fatal().Msg("usage: server assign-legacy-owner <username>")
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

	app := appPkg.New(appPkg.Deps{
		DB:     db,
		Google: auth.NewGoogleVerifier(cfg.Google.ClientID),
		Mailer: mail,
		WebURL: cfg.App.WebURL,
	})

	setupGracefulShutdown(app)

	addr := fmt.Sprintf(":%d", cfg.App.Port)
	log.Info().Str("addr", addr).Msg("server starting")
	if err := app.Listen(addr); err != nil {
		log.Error().Err(err).Msg("server error")
	}
}

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
