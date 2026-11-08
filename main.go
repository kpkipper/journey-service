package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/config"
	"github.com/kpkipper/journey-service/database"
	appPkg "github.com/kpkipper/journey-service/internal/app"
	"github.com/kpkipper/journey-service/internal/auth"
	"github.com/kpkipper/journey-service/internal/mailer"
	"github.com/kpkipper/journey-service/pkg/logger"
)

func main() {
	cfg := config.Get()
	logger.Init(cfg.App.ENV != "production")
	log := logger.Get()

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

	if cfg.Google.ClientID == "" {
		log.Warn().Msg("GOOGLE_CLIENT_ID is empty; Google login will be rejected")
	}

	var mail mailer.Mailer
	if cfg.SMTP.Host == "" {
		if !allowLogMailer(cfg.App.ENV) {
			log.Fatal().Str("app_env", cfg.App.ENV).Msg("SMTP_HOST must be set unless APP_ENV is empty, development or test (the log mailer prints login links)")
		}
		mail = mailer.NewLogMailer()
	} else {
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
	case "", "development", "test":
		return true
	}
	return false
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
