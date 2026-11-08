package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/kpkipper/journey-service/internal/middleware"
	"github.com/kpkipper/journey-service/internal/services"
	"github.com/kpkipper/journey-service/pkg/utils"
)

type MemberHandler struct {
	svc *services.MemberService
}

func NewMemberHandler(svc *services.MemberService) *MemberHandler {
	return &MemberHandler{svc: svc}
}

func (h *MemberHandler) List(c *fiber.Ctx) error {
	out, err := h.svc.ListMembers(c.Context(), middleware.CurrentUserID(c), c.Params("slug"))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, out)
}

func (h *MemberHandler) Invite(c *fiber.Ctx) error {
	var in struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	m, err := h.svc.Invite(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), in.Username, in.Role)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, m)
}

func (h *MemberHandler) ChangeRole(c *fiber.Ctx) error {
	var in struct {
		Role string `json:"role"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	if err := h.svc.ChangeRole(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("username"), in.Role); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}

func (h *MemberHandler) Remove(c *fiber.Ctx) error {
	if err := h.svc.Remove(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("username")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}

func (h *MemberHandler) Leave(c *fiber.Ctx) error {
	if err := h.svc.Leave(c.Context(), middleware.CurrentUserID(c), c.Params("slug")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}

func (h *MemberHandler) ListInviteLinks(c *fiber.Ctx) error {
	out, err := h.svc.ListInviteLinks(c.Context(), middleware.CurrentUserID(c), c.Params("slug"))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, out)
}

func (h *MemberHandler) CreateInviteLink(c *fiber.Ctx) error {
	var in struct {
		Role string `json:"role"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	url, link, err := h.svc.CreateInviteLink(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), in.Role)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusCreated, fiber.Map{"url": url, "link": link})
}

func (h *MemberHandler) RevokeInviteLink(c *fiber.Ctx) error {
	if err := h.svc.RevokeInviteLink(c.Context(), middleware.CurrentUserID(c), c.Params("slug"), c.Params("id")); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}

func (h *MemberHandler) JoinByLink(c *fiber.Ctx) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&in); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid request body")
	}
	slug, err := h.svc.JoinByLink(c.Context(), middleware.CurrentUserID(c), in.Token)
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{"slug": slug})
}

func (h *MemberHandler) MyInvites(c *fiber.Ctx) error {
	out, err := h.svc.MyInvites(c.Context(), middleware.CurrentUserID(c))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, out)
}

func (h *MemberHandler) Accept(c *fiber.Ctx) error  { return h.respond(c, true) }
func (h *MemberHandler) Decline(c *fiber.Ctx) error { return h.respond(c, false) }

func (h *MemberHandler) respond(c *fiber.Ctx, accept bool) error {
	if err := h.svc.RespondInvite(c.Context(), middleware.CurrentUserID(c), c.Params("id"), accept); err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, fiber.Map{})
}

func (h *MemberHandler) MyJourneys(c *fiber.Ctx) error {
	out, err := h.svc.MyJourneys(c.Context(), middleware.CurrentUserID(c))
	if err != nil {
		return respondError(c, err)
	}
	return utils.Success(c, fiber.StatusOK, out)
}
