package ai

import (
	"context"
	"errors"

	"mailess/backend/internal/config"
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
func New(cfg config.Config) *Manager {
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

// DraftReply writes a reply draft given the original email context.
func (m *Manager) DraftReply(ctx context.Context, context string) (string, error) {
	if m.provider == nil {
		return "", ErrDisabled
	}
	return m.provider.Chat(ctx, draftSystem, context)
}

const summarizeSystem = "You are an email assistant. Summarize the following email in 3-4 concise bullet points in the same language as the email. Return only the summary."

const draftSystem = "You are an email assistant. Write a polite reply draft to the email below. Keep it concise and natural, in the same language as the email. Return only the reply body, no subject."
