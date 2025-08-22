package api

import (
	"github.com/gofiber/fiber/v2"
)

func (h *Handler) registerAI(r fiber.Router) {
	r.Get("/ai/status", h.aiStatus)
	r.Post("/ai/summarize", h.aiSummarize)
	r.Post("/ai/draft", h.aiDraft)
}

// aiStatus tells the UI whether AI features are available.
func (h *Handler) aiStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"enabled":  h.AI.Enabled(),
		"provider": h.AI.ProviderName(),
	})
}

// aiSummarize summarizes an email body.
func (h *Handler) aiSummarize(c *fiber.Ctx) error {
	if !h.AI.Enabled() {
		return c.Status(400).JSON(fiber.Map{"error": "ai is disabled"})
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text is required"})
	}
	summary, err := h.AI.Summarize(c.Context(), in.Text)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"summary": summary})
}

// aiDraft writes a reply draft from email context.
func (h *Handler) aiDraft(c *fiber.Ctx) error {
	if !h.AI.Enabled() {
		return c.Status(400).JSON(fiber.Map{"error": "ai is disabled"})
	}
	var in struct {
		Context string `json:"context"`
	}
	if err := c.BodyParser(&in); err != nil || in.Context == "" {
		return c.Status(400).JSON(fiber.Map{"error": "context is required"})
	}
	draft, err := h.AI.DraftReply(c.Context(), in.Context)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"draft": draft})
}
