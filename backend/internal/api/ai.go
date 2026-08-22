package api

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/ai"
	"mailez/backend/internal/mail"
)

func (h *Handler) registerAI(r fiber.Router) {
	r.Get("/ai/status", h.aiStatus)
	r.Post("/ai/summarize", h.aiSummarize)
	r.Post("/ai/draft", h.aiDraft)
	r.Post("/ai/prioritize", h.aiPrioritize)
	r.Post("/ai/search", h.aiSearch)
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

// aiPrioritize scores a batch of emails by importance.
func (h *Handler) aiPrioritize(c *fiber.Ctx) error {
	if !h.AI.Enabled() {
		return c.Status(400).JSON(fiber.Map{"error": "ai is disabled"})
	}
	var in struct {
		Messages []ai.PriorityItem `json:"messages"`
	}
	if err := c.BodyParser(&in); err != nil || len(in.Messages) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "messages are required"})
	}
	scores, err := h.AI.Prioritize(c.Context(), in.Messages)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	out := make(map[string]int, len(scores))
	for uid, s := range scores {
		out[strconv.FormatUint(uint64(uid), 10)] = s
	}
	return c.JSON(fiber.Map{"scores": out})
}

// aiSearch interprets a natural-language query and runs an IMAP search with
// the extracted criteria.
func (h *Handler) aiSearch(c *fiber.Ctx) error {
	if !h.AI.Enabled() {
		return c.Status(400).JSON(fiber.Map{"error": "ai is disabled"})
	}
	var in struct {
		Query string `json:"query"`
	}
	if err := c.BodyParser(&in); err != nil || in.Query == "" {
		return c.Status(400).JSON(fiber.Map{"error": "query is required"})
	}
	spec, err := h.AI.InterpretSearch(c.Context(), in.Query)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}

	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	q := mail.SearchQuery{
		Text:          spec.Keywords,
		HasAttachment: spec.HasAttachment,
	}
	if spec.From != "" {
		q.From = []string{spec.From}
	}
	if spec.To != "" {
		q.To = []string{spec.To}
	}
	if spec.Subject != "" {
		q.Subject = []string{spec.Subject}
	}
	if spec.Before != "" {
		if t, err := time.Parse("2006-01-02", spec.Before); err == nil {
			q.Before = &t
		}
	}
	if spec.After != "" {
		if t, err := time.Parse("2006-01-02", spec.After); err == nil {
			q.After = &t
		}
	}

	folder := c.Query("folder", "INBOX")
	messages, err := h.Mail.SearchMessagesSpec(user.Email, token, folder, q)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"query": in.Query, "messages": messages})
}
