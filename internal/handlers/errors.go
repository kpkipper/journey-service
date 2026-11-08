package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/logger"
	"github.com/kpkipper/journey-service/pkg/utils"
)

// respondError maps service errors to HTTP responses.
func respondError(c *fiber.Ctx, err error) error {
	var ve *services.ValidationError
	switch {
	case errors.As(err, &ve):
		return utils.Error(c, fiber.StatusBadRequest, ve.Error())
	case errors.Is(err, services.ErrInvalidToken):
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	case errors.Is(err, services.ErrUnauthorized):
		return utils.Error(c, fiber.StatusUnauthorized, err.Error())
	case errors.Is(err, services.ErrUserNotFound):
		return utils.Error(c, fiber.StatusNotFound, err.Error())
	case errors.Is(err, services.ErrUsernameTaken):
		return utils.Error(c, fiber.StatusConflict, err.Error())
	case errors.Is(err, services.ErrTooManyRequests):
		return utils.Error(c, fiber.StatusTooManyRequests, err.Error())
	default:
		logger.Get().Error().Err(err).
			Str("request_id", c.GetRespHeader("X-Request-ID")).
			Str("path", c.Path()).
			Msg("unhandled error")
		return utils.Error(c, fiber.StatusInternalServerError, "internal server error")
	}
}
