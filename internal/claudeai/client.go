// Package claudeai implements pkg/ai.Client using the Anthropic Claude API.
// The base URL is configurable so tests can point at an httptest server.
package claudeai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/clcollins/gort/internal/metrics"
	"github.com/clcollins/gort/pkg/ai"
	"github.com/clcollins/gort/pkg/ai/prompt"
)

const (
	defaultBaseURL   = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
	maxTokens        = 4096
)

// Option configures the Claude client.
type Option func(*client)

// WithBaseURL overrides the Anthropic API base URL. Used in tests.
func WithBaseURL(url string) Option {
	return func(c *client) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *client) { c.httpClient = hc }
}

type client struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewClient returns an ai.Client backed by the Anthropic Claude API.
func NewClient(apiKey, model string, opts ...Option) ai.Client {
	c := &client{
		apiKey:     apiKey,
		model:      model,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// messagesRequest is the Anthropic Messages API request body.
type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// messagesResponse is the relevant subset of the Anthropic Messages API response.
type messagesResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *client) callMessages(ctx context.Context, prompt string) (string, error) {
	reqBody := messagesRequest{
		Model:     c.model,
		MaxTokens: maxTokens,
		Messages:  []message{{Role: "user", Content: prompt}},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("claudeai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("claudeai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("claudeai: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("claudeai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claudeai: API status %d: %s", resp.StatusCode, string(respBytes))
	}

	var mr messagesResponse
	if err := json.Unmarshal(respBytes, &mr); err != nil {
		return "", fmt.Errorf("claudeai: unmarshal response: %w", err)
	}
	if mr.Error != nil {
		return "", fmt.Errorf("claudeai: API error %s: %s", mr.Error.Type, mr.Error.Message)
	}
	for _, block := range mr.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("claudeai: no text content in response")
}

// Analyze sends a Flux failure context to Claude and parses the structured response.
func (c *client) Analyze(ctx context.Context, req ai.AnalysisRequest) (*ai.AnalysisResult, error) {
	start := time.Now()
	p := prompt.BuildAnalysisPrompt(req)

	text, err := c.callMessages(ctx, p)
	duration := time.Since(start).Seconds()
	if err != nil {
		metrics.AIRequestsTotal.WithLabelValues("analyze", "error").Inc()
		metrics.AIRequestDurationSeconds.WithLabelValues("analyze").Observe(duration)
		return nil, fmt.Errorf("claudeai: analyze: %w", err)
	}
	metrics.AIRequestsTotal.WithLabelValues("analyze", "success").Inc()
	metrics.AIRequestDurationSeconds.WithLabelValues("analyze").Observe(duration)

	return prompt.ParseAnalysisResponse(text), nil
}

// ValidateIntent sends the runtime state and plan docs to Claude for intent validation.
func (c *client) ValidateIntent(ctx context.Context, req ai.IntentValidationRequest) (*ai.IntentValidationResult, error) {
	start := time.Now()
	p := prompt.BuildIntentPrompt(req)

	text, err := c.callMessages(ctx, p)
	duration := time.Since(start).Seconds()
	if err != nil {
		metrics.AIRequestsTotal.WithLabelValues("validate_intent", "error").Inc()
		metrics.AIRequestDurationSeconds.WithLabelValues("validate_intent").Observe(duration)
		return nil, fmt.Errorf("claudeai: validate intent: %w", err)
	}
	metrics.AIRequestsTotal.WithLabelValues("validate_intent", "success").Inc()
	metrics.AIRequestDurationSeconds.WithLabelValues("validate_intent").Observe(duration)

	return prompt.ParseIntentResponse(text), nil
}
