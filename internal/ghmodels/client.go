// Package ghmodels implements pkg/ai.Client using the GitHub Models inference API.
// The base URL is configurable so tests can point at an httptest server.
package ghmodels

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
	defaultBaseURL   = "https://models.github.ai"
	githubAPIVersion = "2025-04-01"
	maxTokens        = 4096
)

// Option configures the GitHub Models client.
type Option func(*client)

// WithBaseURL overrides the GitHub Models API base URL. Used in tests.
func WithBaseURL(url string) Option {
	return func(c *client) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *client) { c.httpClient = hc }
}

type client struct {
	token      string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewClient returns an ai.Client backed by the GitHub Models inference API.
// The token should be a GitHub PAT with models:read scope.
func NewClient(token, model string, opts ...Option) ai.Client {
	c := &client{
		token:      token,
		model:      model,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// chatRequest is the OpenAI-compatible chat completions request body.
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the relevant subset of the OpenAI-compatible chat completions response.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *client) callChat(ctx context.Context, userPrompt string) (string, error) {
	reqBody := chatRequest{
		Model:     c.model,
		Messages:  []chatMessage{{Role: "user", Content: userPrompt}},
		MaxTokens: maxTokens,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ghmodels: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/inference/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ghmodels: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ghmodels: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ghmodels: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ghmodels: API status %d: %s", resp.StatusCode, string(respBytes))
	}

	var cr chatResponse
	if err := json.Unmarshal(respBytes, &cr); err != nil {
		return "", fmt.Errorf("ghmodels: unmarshal response: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("ghmodels: API error %s: %s", cr.Error.Type, cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("ghmodels: no choices in response")
	}
	return cr.Choices[0].Message.Content, nil
}

// Analyze sends a Flux failure context to GitHub Models and parses the structured response.
func (c *client) Analyze(ctx context.Context, req ai.AnalysisRequest) (*ai.AnalysisResult, error) {
	start := time.Now()
	p := prompt.BuildAnalysisPrompt(req)

	text, err := c.callChat(ctx, p)
	duration := time.Since(start).Seconds()
	if err != nil {
		metrics.AIRequestsTotal.WithLabelValues("analyze", "error").Inc()
		metrics.AIRequestDurationSeconds.WithLabelValues("analyze").Observe(duration)
		return nil, fmt.Errorf("ghmodels: analyze: %w", err)
	}
	metrics.AIRequestsTotal.WithLabelValues("analyze", "success").Inc()
	metrics.AIRequestDurationSeconds.WithLabelValues("analyze").Observe(duration)

	return prompt.ParseAnalysisResponse(text), nil
}

// ValidateIntent sends the runtime state and plan docs to GitHub Models for intent validation.
func (c *client) ValidateIntent(ctx context.Context, req ai.IntentValidationRequest) (*ai.IntentValidationResult, error) {
	start := time.Now()
	p := prompt.BuildIntentPrompt(req)

	text, err := c.callChat(ctx, p)
	duration := time.Since(start).Seconds()
	if err != nil {
		metrics.AIRequestsTotal.WithLabelValues("validate_intent", "error").Inc()
		metrics.AIRequestDurationSeconds.WithLabelValues("validate_intent").Observe(duration)
		return nil, fmt.Errorf("ghmodels: validate intent: %w", err)
	}
	metrics.AIRequestsTotal.WithLabelValues("validate_intent", "success").Inc()
	metrics.AIRequestDurationSeconds.WithLabelValues("validate_intent").Observe(duration)

	return prompt.ParseIntentResponse(text), nil
}
