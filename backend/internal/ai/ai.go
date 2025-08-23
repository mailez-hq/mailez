package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mailez/backend/internal/core"
)

// ErrDisabled is returned when no AI provider is configured.
var ErrDisabled = errors.New("ai is disabled")

// Provider is a pluggable LLM backend. Webmail core depends only on this
// interface; providers are registered via configuration.
type Provider interface {
	Name() string
	Chat(ctx context.Context, system, user string) (string, error)
}

// Manager dispatches AI features to the configured provider. A nil provider
// means AI is fully disabled (zero cost, no external calls).
type Manager struct {
	provider Provider
}

// New builds a manager from configuration. AI_PROVIDER=none disables AI.
func New(cfg core.Config) *Manager {
	switch cfg.AIProvider {
	case "openai":
		return &Manager{provider: NewOpenAI(cfg)}
	default:
		return &Manager{}
	}
}

// Enabled reports whether an AI provider is configured.
func (m *Manager) Enabled() bool {
	return m.provider != nil
}

// ProviderName returns the active provider name, or "none".
func (m *Manager) ProviderName() string {
	if m.provider == nil {
		return "none"
	}
	return m.provider.Name()
}

// Summarize produces a concise summary of an email body.
func (m *Manager) Summarize(ctx context.Context, text string) (string, error) {
	if m.provider == nil {
		return "", ErrDisabled
	}
	return m.provider.Chat(ctx, summarizeSystem, text)
}

// DraftTone controls the style of an AI-generated reply draft. An empty tone
// falls back to the default (formal) behavior.
type DraftTone string

const (
	ToneFormal   DraftTone = "formal"
	ToneConcise  DraftTone = "concise"
	ToneFriendly DraftTone = "friendly"
)

// DraftReply writes a reply draft given the original email context.
func (m *Manager) DraftReply(ctx context.Context, tone DraftTone, context string) (string, error) {
	if m.provider == nil {
		return "", ErrDisabled
	}
	system := draftSystem
	switch tone {
	case ToneConcise:
		system = draftConciseSystem
	case ToneFriendly:
		system = draftFriendlySystem
	}
	return m.provider.Chat(ctx, system, context)
}

// PriorityItem is a lightweight mail descriptor sent for AI ranking.
type PriorityItem struct {
	UID     uint32 `json:"uid"`
	Subject string `json:"subject"`
	From    string `json:"from"`
}

// PriorityCategory is one of the coarse inbox buckets the AI may assign.
type PriorityCategory string

const (
	CategoryWork       PriorityCategory = "work"
	CategoryNewsletter PriorityCategory = "newsletter"
	CategorySocial     PriorityCategory = "social"
	CategoryShopping   PriorityCategory = "shopping"
	CategoryFinance    PriorityCategory = "finance"
	CategoryOther      PriorityCategory = "other"
)

// PriorityResult pairs an importance score with an inbox category per uid.
type PriorityResult struct {
	Scores     map[uint32]int
	Categories map[uint32]PriorityCategory
}

// SearchSpec is the structured interpretation of a natural-language search
// request such as "emails with attachments from Amy last month".
type SearchSpec struct {
	Keywords      []string `json:"keywords"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Subject       string   `json:"subject"`
	HasAttachment bool     `json:"has_attachment"`
	Before        string   `json:"before"` // YYYY-MM-DD, optional
	After         string   `json:"after"`  // YYYY-MM-DD, optional
}

// Prioritize scores a batch of emails from 1 (lowest) to 5 (highest) and
// buckets each into a PriorityCategory with a single provider call returning a
// JSON object keyed by uid.
func (m *Manager) Prioritize(ctx context.Context, items []PriorityItem) (PriorityResult, error) {
	if m.provider == nil {
		return PriorityResult{}, ErrDisabled
	}
	var b strings.Builder
	b.WriteString("Rate each email below by importance for a busy professional, from 1 (lowest) to 5 (highest), and classify each into exactly one of: work, newsletter, social, shopping, finance, other. Consider the sender relationship, urgency and subject.\n\n")
	for _, it := range items {
		fmt.Fprintf(&b, "- uid=%d from=%q subject=%q\n", it.UID, it.From, it.Subject)
	}
	b.WriteString("\nRespond ONLY with a JSON object mapping uid to {\"score\": 1-5, \"category\": \"work\"}, e.g. {\"12\": {\"score\": 5, \"category\": \"work\"}, \"3\": {\"score\": 2, \"category\": \"newsletter\"}}.")

	raw, err := m.provider.Chat(ctx, prioritySystem, b.String())
	if err != nil {
		return PriorityResult{}, err
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return PriorityResult{}, fmt.Errorf("ai prioritize: unexpected response %q", raw)
	}
	var parsed map[string]struct {
		Score    int    `json:"score"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return PriorityResult{}, fmt.Errorf("ai prioritize: parse: %w", err)
	}
	res := PriorityResult{
		Scores:     make(map[uint32]int, len(parsed)),
		Categories: make(map[uint32]PriorityCategory, len(parsed)),
	}
	for k, v := range parsed {
		var uid uint64
		if _, err := fmt.Sscanf(k, "%d", &uid); err != nil {
			continue
		}
		score := v.Score
		if score < 1 {
			score = 1
		} else if score > 5 {
			score = 5
		}
		res.Scores[uint32(uid)] = score
		if v.Category != "" {
			res.Categories[uint32(uid)] = PriorityCategory(v.Category)
		}
	}
	return res, nil
}

// InterpretSearch translates a natural-language search into structured
// criteria. Dates use YYYY-MM-DD; relative phrases ("last month") are
// resolved by the model.
func (m *Manager) InterpretSearch(ctx context.Context, query string) (SearchSpec, error) {
	if m.provider == nil {
		return SearchSpec{}, ErrDisabled
	}
	raw, err := m.provider.Chat(ctx, searchSystem, query)
	if err != nil {
		return SearchSpec{}, err
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var spec SearchSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return SearchSpec{}, fmt.Errorf("ai search: parse: %w", err)
	}
	return spec, nil
}

const summarizeSystem = "You are an email assistant. Summarize the following email in 3-4 concise bullet points in the same language as the email. Return only the summary."

const draftSystem = "You are an email assistant. Write a polite reply draft to the email below. Keep it concise and natural, in the same language as the email. Return only the reply body, no subject."

const draftConciseSystem = "You are an email assistant. Write a short, direct reply draft to the email below. Use as few words as possible without being rude, in the same language as the email. Return only the reply body, no subject."

const draftFriendlySystem = "You are an email assistant. Write a warm, friendly reply draft to the email below, in the same language as the email. Return only the reply body, no subject."

const prioritySystem = "You are an email prioritization assistant. Return only a JSON object mapping each uid to {\"score\": 1-5, \"category\": \"work|newsletter|social|shopping|finance|other\"}."

const searchSystem = `You translate natural-language email search requests into structured criteria.
Return ONLY JSON with this exact shape:
{"keywords":[],"from":"","to":"","subject":"","has_attachment":false,"before":"YYYY-MM-DD","after":"YYYY-MM-DD"}
Rules: keywords are plain search terms; from/to/subject are exact-ish matchers (empty string when unknown); has_attachment true only when the user asks for attachments; before/after use YYYY-MM-DD and resolve relative dates ("last month", "yesterday") to concrete dates; omit (empty) what is not mentioned.`
