package models

// AiConfig holds the AI provider settings configured through the admin
// console. Stored as a single row (id = 1). The API key is encrypted with
// the control-plane secret (crypto.Encrypt), like fetch passwords.
type AiConfig struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Enabled bool   `json:"enabled"`
	// Provider is the LLM backend name (only "openai" is implemented today).
	Provider string `gorm:"size:32" json:"provider"`
	// BaseURL is the OpenAI-compatible endpoint, e.g. https://api.deepseek.com
	// or http://host:11434/v1 (Ollama). Empty defaults to api.openai.com/v1.
	BaseURL string `gorm:"size:255" json:"base_url"`
	// APIKeyEnc holds the encrypted API key; never exposed in GET responses.
	APIKeyEnc string `gorm:"size:1024" json:"-"`
	// Model is the completion model name, e.g. deepseek-chat / gpt-4o-mini.
	Model string `gorm:"size:128" json:"model"`
}
