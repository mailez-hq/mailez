package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"

	"gorm.io/gorm"
)

// ErrDisabled is returned when no AI provider is configured.
var ErrDisabled = errors.New("ai is disabled")

// Provider is a pluggable LLM backend. Webmail core depends only on this
// interface; providers are registered via configuration.
type Provider interface {
	Name() string
	Chat(ctx context.Context, system, user string) (string, error)
}

// Manager dispatches AI features to the configured provider. The provider is
// resolved from the database on every call, so admin-side configuration
// changes apply without a restart. A nil provider means AI is fully disabled.
type Manager struct {
	db        *gorm.DB
	secretKey string
	// envFallback mirrors the legacy AI_PROVIDER=... environment variables
	// for the very first boot, before an admin saves a DB row.
	envFallback Provider
}

// New builds a manager backed by the database. The legacy environment
// variables only seed the first-boot fallback; the admin console is the
// source of truth once a row exists.
func New(db *gorm.DB, cfg core.Config) *Manager {
	m := &Manager{db: db, secretKey: cfg.SecretKey}
	switch cfg.AIProvider {
	case "openai":
		m.envFallback = NewOpenAI(cfg)
	}
	return m
}

// load returns the active provider, preferring the database configuration.
func (m *Manager) load() Provider {
	if m.db != nil {
		var row models.AiConfig
		if err := m.db.First(&row).Error; err == nil {
			if !row.Enabled || row.Provider == "" {
				return nil
			}
			key := ""
			if row.APIKeyEnc != "" {
				if k, err := crypto.Decrypt(m.secretKey, row.APIKeyEnc); err == nil {
					key = k
				}
			}
			base := row.BaseURL
			if base == "" {
				base = "https://api.openai.com/v1"
			}
			model := row.Model
			if model == "" {
				model = "gpt-4o-mini"
			}
			switch row.Provider {
			case "openai":
				return NewOpenAIProvider(base, key, model)
			}
			return nil
		}
	}
	if m.envFallback != nil {
		return m.envFallback
	}
	return nil
}

// Enabled reports whether an AI provider is configured.
func (m *Manager) Enabled() bool {
	return m.load() != nil
}

// ProviderName returns the active provider name, or "none".
func (m *Manager) ProviderName() string {
	p := m.load()
	if p == nil {
		return "none"
	}
	return p.Name()
}

// Summarize produces a concise summary of an email body.
func (m *Manager) Summarize(ctx context.Context, text string) (string, error) {
	p := m.load()
	if p == nil {
		return "", ErrDisabled
	}
	return p.Chat(ctx, summarizeSystem, text)
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
	p := m.load()
	if p == nil {
		return "", ErrDisabled
	}
	system := draftSystem
	switch tone {
	case ToneConcise:
		system = draftConciseSystem
	case ToneFriendly:
		system = draftFriendlySystem
	}
	return p.Chat(ctx, system, context)
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
	p := m.load()
	if p == nil {
		return PriorityResult{}, ErrDisabled
	}
	var b strings.Builder
	b.WriteString("Rate each email below by importance for a busy professional, from 1 (lowest) to 5 (highest), and classify each into exactly one of: work, newsletter, social, shopping, finance, other. Consider the sender relationship, urgency and subject.\n\n")
	for _, it := range items {
		fmt.Fprintf(&b, "- uid=%d from=%q subject=%q\n", it.UID, it.From, it.Subject)
	}
	b.WriteString("\nRespond ONLY with a JSON object mapping uid to {\"score\": 1-5, \"category\": \"work\"}, e.g. {\"12\": {\"score\": 5, \"category\": \"work\"}, \"3\": {\"score\": 2, \"category\": \"newsletter\"}}.")

	raw, err := p.Chat(ctx, prioritySystem, b.String())
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
	p := m.load()
	if p == nil {
		return SearchSpec{}, ErrDisabled
	}
	raw, err := p.Chat(ctx, searchSystem, query)
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
