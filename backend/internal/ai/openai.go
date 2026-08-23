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

func (o *OpenAI) Name() string { return "openai" }

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
	body, err := json.Marshal(chatRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.3,
		MaxTokens:   500,
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
