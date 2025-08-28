package ai

import (
	"bufio"
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
	Stream      bool          `json:"stream,omitempty"`
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

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// ChatStream calls the chat completions endpoint with stream: true and
// invokes onDelta for every content token as it arrives.
func (o *OpenAI) ChatStream(ctx context.Context, system, user string, onDelta func(string)) error {
	body, err := json.Marshal(chatRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.3,
		MaxTokens:   800,
		Stream:      true,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ai request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ai upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				break
			}
			continue
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onDelta(chunk.Choices[0].Delta.Content)
		}
	}
	return scanner.Err()
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
