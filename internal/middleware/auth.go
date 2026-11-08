package middleware

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/logger"
	"github.com/kpkipper/journey-service/pkg/utils"
)

const userIDKey = "user_id"

// BearerToken extracts the token from an "Authorization: Bearer <token>" header (scheme is case-insensitive).
func BearerToken(c *fiber.Ctx) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(c.Get(fiber.HeaderAuthorization)), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t") {
		return ""
	}
	return token
}

// RequireAuth rejects requests without a valid session token; apply it per route.
func RequireAuth(svc *services.AuthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := BearerToken(c)
		if token == "" {
			return utils.Error(c, fiber.StatusUnauthorized, "unauthorized")
		}
		user, err := svc.Authenticate(c.Context(), token)
		if err != nil {
			if errors.Is(err, services.ErrUnauthorized) {
				return utils.Error(c, fiber.StatusUnauthorized, "unauthorized")
			}
			logger.Get().Error().Err(err).
				Str("request_id", c.GetRespHeader("X-Request-ID")).
				Str("path", c.Path()).
				Msg("authenticate failed")
			return utils.Error(c, fiber.StatusInternalServerError, "internal server error")
		}
		c.Locals(userIDKey, user.ID)
		return c.Next()
	}
}

// CurrentUserID returns the authenticated user's ID set by RequireAuth.
func CurrentUserID(c *fiber.Ctx) string {
	id, _ := c.Locals(userIDKey).(string)
	return id
}
