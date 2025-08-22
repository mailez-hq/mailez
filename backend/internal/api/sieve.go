package api

import (
	"github.com/gofiber/fiber/v2"
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

func (h *Handler) sieveList(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	scripts, err := h.Mail.SieveListScripts(email, token)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(scripts)
}

func (h *Handler) sieveGet(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	content, err := h.Mail.SieveGetScript(email, token, c.Params("name"))
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"name": c.Params("name"), "content": content})
}

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
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) sieveDelete(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	if err := h.Mail.SieveDeleteScript(email, token, c.Params("name")); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) sieveActivate(c *fiber.Ctx) error {
	email, token, err := h.sieveAuth(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	if err := h.Mail.SieveSetActive(email, token, c.Params("name")); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
