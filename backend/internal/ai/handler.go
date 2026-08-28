package ai

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/mail"
)

// Handler serves the AI domain routes.
type Handler struct {
	*core.App
	AI *Manager
}

var currentUser = core.CurrentUser

func (h *Handler) mailToken(c *fiber.Ctx) (string, error) { return h.App.MailToken(c) }

// RegisterAPI mounts the AI routes.
func RegisterAPI(r fiber.Router, app *core.App, mgr *Manager) {
	h := &Handler{app, mgr}
	h.registerAI(r)
}

func (h *Handler) registerAI(r fiber.Router) {
	r.Get("/ai/status", h.aiStatus)
	r.Post("/ai/summarize", h.aiSummarize)
	r.Post("/ai/draft", h.aiDraft)
	r.Post("/ai/prioritize", h.aiPrioritize)
	r.Post("/ai/search", h.aiSearch)
	r.Post("/ai/translate", h.aiTranslate)
	r.Post("/ai/replies", h.aiReplies)
}

// aiReplies returns short ready-to-send reply suggestions (Smart Reply).
// @Summary Suggest quick replies
// @Tags ai
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /ai/replies [post]
func (h *Handler) aiReplies(c *fiber.Ctx) error {
	if !h.AI.Enabled() {
		return c.Status(400).JSON(fiber.Map{"error": "ai is disabled"})
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&in); err != nil || strings.TrimSpace(in.Text) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text is required"})
	}
	replies, err := h.AI.SmartReplies(c.Context(), in.Text)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"replies": replies})
}

// aiTranslate translates an email body into a target language via the
// configured AI provider.
// @Summary Translate email
// @Tags ai
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /ai/translate [post]
func (h *Handler) aiTranslate(c *fiber.Ctx) error {
	if !h.AI.Enabled() {
		return c.Status(400).JSON(fiber.Map{"error": "ai is disabled"})
	}
	var in struct {
		Text   string `json:"text"`
		Target string `json:"target"`
	}
	if err := c.BodyParser(&in); err != nil || in.Text == "" {
		return c.Status(400).JSON(fiber.Map{"error": "text is required"})
	}
	if in.Target == "" {
		in.Target = "简体中文"
	}
	translated, err := h.AI.Translate(c.Context(), in.Text, in.Target)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"translation": translated})
}

// aiStatus tells the UI whether AI features are available.
// aiStatus reports whether an AI provider is configured.
// @Summary AI status
// @Tags ai
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /ai/status [get]
func (h *Handler) aiStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"enabled":  h.AI.Enabled(),
		"provider": h.AI.ProviderName(),
	})
}

// aiSummarize summarizes an email body.
// aiSummarize produces a summary of an email.
// @Summary Summarize email
// @Tags ai
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /ai/summarize [post]
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
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"summary": summary})
}

// aiDraft writes a reply draft from email context. An optional tone selects
// the writing style; see ai.DraftTone.
// aiDraft writes a reply draft in a chosen tone.
// @Summary Draft reply
// @Tags ai
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /ai/draft [post]
func (h *Handler) aiDraft(c *fiber.Ctx) error {
	if !h.AI.Enabled() {
		return c.Status(400).JSON(fiber.Map{"error": "ai is disabled"})
	}
	var in struct {
		Context string `json:"context"`
		Subject string `json:"subject"`
		Hint    string `json:"hint"`
		Tone    string `json:"tone"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	var draft string
	var err error
	switch {
	case strings.TrimSpace(in.Context) != "":
		// Reply/forward: the original email body is the context.
		draft, err = h.AI.DraftReply(c.Context(), DraftTone(in.Tone), in.Context)
	case strings.TrimSpace(in.Subject) != "" || strings.TrimSpace(in.Hint) != "":
		// New mail: write from the subject + sender's hints.
		draft, err = h.AI.DraftNew(c.Context(), DraftTone(in.Tone), in.Subject, in.Hint)
	default:
		return c.Status(400).JSON(fiber.Map{"error": "context or subject is required"})
	}
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"draft": draft})
}

// aiPrioritize scores a batch of emails by importance.
// aiPrioritize scores a batch of emails by importance.
// @Summary Prioritize inbox
// @Tags ai
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} models.APIError
// @Router /ai/prioritize [post]
func (h *Handler) aiPrioritize(c *fiber.Ctx) error {
	if !h.AI.Enabled() {
		return c.Status(400).JSON(fiber.Map{"error": "ai is disabled"})
	}
	var in struct {
		Messages []PriorityItem `json:"messages"`
	}
	if err := c.BodyParser(&in); err != nil || len(in.Messages) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "messages are required"})
	}
	scores, err := h.AI.Prioritize(c.Context(), in.Messages)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	outScores := make(map[string]int, len(scores.Scores))
	for uid, s := range scores.Scores {
		outScores[strconv.FormatUint(uint64(uid), 10)] = s
	}
	outCategories := make(map[string]string, len(scores.Categories))
	for uid, cat := range scores.Categories {
		outCategories[strconv.FormatUint(uint64(uid), 10)] = string(cat)
	}
	return c.JSON(fiber.Map{"scores": outScores, "categories": outCategories})
}

// aiSearch interprets a natural-language query and runs an IMAP search with
// the extracted criteria.
// aiSearch interprets a natural-language query and runs an IMAP search.
// @Summary AI search
// @Tags ai
// @Accept json
// @Produce json
// @Success 200 {array} mail.Message
// @Failure 400 {object} models.APIError
// @Router /ai/search [post]
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
		return core.Fail(c, 502, err, "mail service error")
	}

	d, err := h.MailDial(c)
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
	messages, err := h.Mail.With(d).SearchMessagesSpec(d.Email, d.Token, folder, q)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"query": in.Query, "messages": messages})
}
