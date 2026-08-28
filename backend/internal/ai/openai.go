package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mailez/backend/internal/core"
)

// OpenAI is an OpenAI-compatible chat completions provider. Any server that
// speaks /chat/completions (OpenAI, Ollama, vLLM, ...) works.
type OpenAI struct {
	baseURL string
	apiKey  string
	model   string
}

// NewOpenAI creates the provider from configuration.
func NewOpenAI(cfg core.Config) *OpenAI {
	return &OpenAI{
		baseURL: strings.TrimRight(cfg.AIBaseURL, "/"),
		apiKey:  cfg.AIAPIKey,
		model:   cfg.AIModel,
	}
}

// NewOpenAIProvider builds an OpenAI-compatible provider from explicit
// settings (database-backed admin configuration).
func NewOpenAIProvider(baseURL, apiKey, model string) *OpenAI {
	return &OpenAI{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
	}
}

func (o *OpenAI) Name() string { return "openai" }

// Ping verifies the endpoint + key + model work end to end with a minimal
// one-token request. Used by the admin connection test.
func (o *OpenAI) Ping(ctx context.Context) error {
	_, err := o.chat(ctx, "ping", "ping", 1)
	return err
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Chat calls the chat completions endpoint.
func (o *OpenAI) Chat(ctx context.Context, system, user string) (string, error) {
	return o.chat(ctx, system, user, 500)
}

func (o *OpenAI) chat(ctx context.Context, system, user string, maxTokens int) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("ai upstream returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}
