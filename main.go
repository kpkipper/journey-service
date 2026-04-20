package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/config"
	"github.com/kpkipper/journey-service/database"
	"github.com/kpkipper/journey-service/internal/handlers"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/repository"
	"github.com/kpkipper/journey-service/internal/routes"
	"github.com/kpkipper/journey-service/internal/services"
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

	repo := repository.NewJourneyRepository(db)
	svc := services.NewJourneyService(repo)
	handler := handlers.NewJourneyHandler(svc)

	app := fiber.New(fiber.Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})

	middleware.Register(app)
	routes.Register(app, handler)

	setupGracefulShutdown(app)

	addr := fmt.Sprintf(":%d", cfg.App.Port)
	log.Info().Str("addr", addr).Msg("server starting")
	if err := app.Listen(addr); err != nil {
		log.Error().Err(err).Msg("server error")
	}
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
