package sieve

import (
	"github.com/gofiber/fiber/v2"
	"mailez/backend/internal/core"
)

func (h *Handler) registerSieve(r fiber.Router) {
	r.Get("/sieve", h.sieveList)
	r.Get("/sieve/:name", h.sieveGet)
	r.Put("/sieve/:name", h.sievePut)
	r.Delete("/sieve/:name", h.sieveDelete)
	r.Post("/sieve/:name/activate", h.sieveActivate)
}

func (h *Handler) sieveAuth(c *fiber.Ctx) (email, token string, err error) {
	user := currentUser(c)
	token, err = h.mailToken(c)
	if err != nil {
		return "", "", err
	}
	return user.Email, token, nil
}

// sieveList returns the user's Sieve scripts.
// @Summary List Sieve scripts
// @Tags sieve
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Router /sieve [get]
func (h *Handler) sieveList(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	scripts, err := h.Mail.SieveListScripts(email, token)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(scripts)
}

// sieveGet returns one Sieve script's content.
// @Summary Get Sieve script
// @Tags sieve
// @Produce json
// @Param name path string true "script name"
// @Success 200 {object} map[string]interface{}
// @Router /sieve/{name} [get]
func (h *Handler) sieveGet(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	content, err := h.Mail.SieveGetScript(email, token, c.Params("name"))
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"name": c.Params("name"), "content": content})
}

// sievePut saves a Sieve script, optionally activating it.
// @Summary Save Sieve script
// @Tags sieve
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /sieve/{name} [put]
func (h *Handler) sievePut(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Content  string `json:"content"`
		Activate bool   `json:"activate"`
	}
	if err := c.BodyParser(&in); err != nil || in.Content == "" {
		return c.Status(400).JSON(fiber.Map{"error": "content is required"})
	}
	if err := h.Mail.SievePutScript(email, token, c.Params("name"), in.Content, in.Activate); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// sieveDelete removes a Sieve script.
// @Summary Delete Sieve script
// @Tags sieve
// @Param name path string true "script name"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /sieve/{name} [delete]
func (h *Handler) sieveDelete(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	if err := h.Mail.SieveDeleteScript(email, token, c.Params("name")); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// sieveActivate makes a script the active filter.
// @Summary Activate Sieve script
// @Tags sieve
// @Param name path string true "script name"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /sieve/{name}/activate [post]
func (h *Handler) sieveActivate(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	if err := h.Mail.SieveSetActive(email, token, c.Params("name")); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
