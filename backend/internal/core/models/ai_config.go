package models

import "time"

// AiConfig holds one AI provider settings row configured through the admin
// console. Multiple providers can coexist; the webmail AI features use the
// enabled provider marked as default (falling back to any enabled provider).
// A provider can only be enabled after its connection test passed; changing
// connection settings clears the test result and disables the row. The API
// key is encrypted with the control-plane secret (crypto.Encrypt), like fetch
// passwords.
type AiConfig struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Name is the admin-facing display name, e.g. "DeepSeek 生产".
	Name    string `gorm:"size:64;not null;default:''" json:"name"`
	Enabled bool   `json:"enabled"`
	// IsDefault marks the provider used when several are enabled. Only an
	// enabled (tested) provider may be the default.
	IsDefault bool `gorm:"not null;default:false" json:"is_default"`
	// Provider is the LLM backend name (only "openai" is implemented today).
	Provider string `gorm:"size:32" json:"provider"`
	// BaseURL is the OpenAI-compatible endpoint, e.g. https://api.deepseek.com
	// or http://host:11434/v1 (Ollama). Empty defaults to api.openai.com/v1.
	BaseURL string `gorm:"size:255" json:"base_url"`
	// APIKeyEnc holds the encrypted API key; never exposed in GET responses.
	APIKeyEnc string `gorm:"size:1024" json:"-"`
	// Model is the completion model name, e.g. deepseek-chat / gpt-4o-mini.
	Model string `gorm:"size:128" json:"model"`
	// LastTestAt is when the connection test last ran; LastTestOK reports
	// whether it passed. Only a passing test allows Enabled to be true.
	LastTestAt    *time.Time `json:"last_test_at"`
	LastTestOK    bool       `gorm:"not null;default:false" json:"last_test_ok"`
	LastTestError string     `gorm:"size:512;not null;default:''" json:"last_test_error"`
}
