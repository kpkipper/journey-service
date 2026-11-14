package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type AuthHandler struct {
	svc *services.AuthService
}

func NewAuthHandler(svc *services.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Google(c *fiber.Ctx) error {
	var in struct {
		IDToken string `json:"id_token"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	token, user, err := h.svc.LoginWithGoogle(c.Context(), in.IDToken)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"token": token, "user": user})
}

// Methods tells the web app which login buttons to show.
func (h *AuthHandler) Methods(c *fiber.Ctx) error {
	email, google := h.svc.Methods()
	return utils.Success(c, fiber.StatusOK, fiber.Map{"email": email, "google": google})
}

func (h *AuthHandler) EmailStart(c *fiber.Ctx) error {
	var in struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.svc.StartEmailLogin(c.Context(), in.Email); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}

func (h *AuthHandler) EmailVerify(c *fiber.Ctx) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	token, user, err := h.svc.VerifyEmailLogin(c.Context(), in.Token)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"token": token, "user": user})
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	if err := h.svc.Logout(c.Context(), middleware.BearerToken(c)); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}
