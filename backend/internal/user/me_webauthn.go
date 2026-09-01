package user

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/webauthn"
)

// registerWebauthn mounts the self-service passkey (WebAuthn) endpoints.
// Registration requires an authenticated session; login through the
// registered credentials happens via /sso/passkey/* without a password.
func (h *Handler) registerWebauthn(r fiber.Router) {
	r.Get("/me/webauthn", h.webauthnList)
	r.Post("/me/webauthn/register/begin", h.webauthnRegisterBegin)
	r.Post("/me/webauthn/register/finish", h.webauthnRegisterFinish)
	r.Delete("/me/webauthn/:id", h.webauthnDelete)
}

// webauthnService builds the ceremony service for the deployment's
// relying-party configuration. Errors mean the deployment has not
// configured passkeys (missing origins).
func (h *Handler) webauthnService() (*webauthn.Service, error) {
	return webauthn.New(h.DB, h.Auth.Store, h.Cfg.WebAuthnRPID, "Mailez", h.Cfg.WebAuthnOrigins)
}

// webauthnList returns the account's registered passkeys.
// @Summary List passkeys
// @Tags me
// @Produce json
// @Success 200 {array} models.WebauthnCredential
// @Router /me/webauthn [get]
func (h *Handler) webauthnList(c *fiber.Ctx) error {
	u := currentUser(c)
	svc, err := h.webauthnService()
	if err != nil {
		return c.JSON(fiber.Map{"credentials": []any{}})
	}
	rows, err := svc.List(u.Email)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(fiber.Map{"credentials": rows})
}

// webauthnRegisterBegin starts the credential-creation ceremony.
// @Summary Begin passkey registration
// @Tags me
// @Accept json
// @Produce json
// @Param body body object true "name"
// @Success 200 {object} map[string]any "options"
// @Router /me/webauthn/register/begin [post]
func (h *Handler) webauthnRegisterBegin(c *fiber.Ctx) error {
	u := currentUser(c)
	svc, err := h.webauthnService()
	if err != nil {
		return core.Fail(c, 400, err, "passkeys are not configured on this deployment")
	}
	options, err := svc.BeginRegistration(u.Email)
	if err != nil {
		if errors.Is(err, webauthn.ErrUnavailable) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "account unavailable"})
		}
		return core.Fail(c, 400, err, "passkey ceremony failed")
	}
	return c.JSON(fiber.Map{"options": options})
}

// webauthnRegisterFinish verifies the attestation and stores the credential.
// @Summary Complete passkey registration
// @Tags me
// @Accept json
// @Produce json
// @Param body body object true "name + credential"
// @Success 200 {object} map[string]string
// @Router /me/webauthn/register/finish [post]
func (h *Handler) webauthnRegisterFinish(c *fiber.Ctx) error {
	u := currentUser(c)
	svc, err := h.webauthnService()
	if err != nil {
		return core.Fail(c, 400, err, "passkeys are not configured on this deployment")
	}
	var in struct {
		Name string `json:"name"`
	}
	_ = c.BodyParser(&in) // the same body carries the credential JSON
	row, err := svc.FinishRegistration(u.Email, in.Name, c.Body())
	if err != nil {
		return core.Fail(c, 400, err, "passkey verification failed")
	}
	return c.JSON(fiber.Map{"id": row.ID, "name": row.Name})
}

// webauthnDelete removes one passkey.
// @Summary Delete a passkey
// @Tags me
// @Param id path int true "credential id"
// @Success 204
// @Router /me/webauthn/{id} [delete]
func (h *Handler) webauthnDelete(c *fiber.Ctx) error {
	u := currentUser(c)
	svc, err := h.webauthnService()
	if err != nil {
		return core.Fail(c, 400, err, "passkeys are not configured on this deployment")
	}
	var id uint
	if v, perr := strconv.ParseUint(c.Params("id"), 10, 64); perr != nil || v == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid id"})
	} else {
		id = uint(v)
	}
	if err := svc.Delete(u.Email, id); err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
