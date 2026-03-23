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
	"github.com/clcollins/gort/pkg/gitops"
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
	defer resp.Body.Close()

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
	prompt := buildAnalysisPrompt(req)

	text, err := c.callMessages(ctx, prompt)
	duration := time.Since(start).Seconds()
	if err != nil {
		metrics.AIRequestsTotal.WithLabelValues("analyze", "error").Inc()
		metrics.AIRequestDurationSeconds.WithLabelValues("analyze").Observe(duration)
		return nil, fmt.Errorf("claudeai: analyze: %w", err)
	}
	metrics.AIRequestsTotal.WithLabelValues("analyze", "success").Inc()
	metrics.AIRequestDurationSeconds.WithLabelValues("analyze").Observe(duration)

	return parseAnalysisResponse(text), nil
}

// ValidateIntent sends the runtime state and plan docs to Claude for intent validation.
func (c *client) ValidateIntent(ctx context.Context, req ai.IntentValidationRequest) (*ai.IntentValidationResult, error) {
	start := time.Now()
	prompt := buildIntentPrompt(req)

	text, err := c.callMessages(ctx, prompt)
	duration := time.Since(start).Seconds()
	if err != nil {
		metrics.AIRequestsTotal.WithLabelValues("validate_intent", "error").Inc()
		metrics.AIRequestDurationSeconds.WithLabelValues("validate_intent").Observe(duration)
		return nil, fmt.Errorf("claudeai: validate intent: %w", err)
	}
	metrics.AIRequestsTotal.WithLabelValues("validate_intent", "success").Inc()
	metrics.AIRequestDurationSeconds.WithLabelValues("validate_intent").Observe(duration)

	return parseIntentResponse(text), nil
}

// buildAnalysisPrompt constructs the prompt for a Flux failure analysis. Pure function.
func buildAnalysisPrompt(req ai.AnalysisRequest) string {
	var b strings.Builder
	b.WriteString("You are GORT, a GitOps reconciliation assistant. A Flux deployment has failed. ")
	b.WriteString("Analyze the failure and propose a fix.\n\n")

	if req.ReconcileStatus != nil {
		fmt.Fprintf(&b, "## Flux Status\nApp: %s/%s\nReady: %v\nReason: %s\nMessage: %s\n\n",
			req.ReconcileStatus.Namespace, req.ReconcileStatus.Name,
			req.ReconcileStatus.Ready, req.ReconcileStatus.Reason, req.ReconcileStatus.Message)
	}

	if len(req.FailureLogs) > 0 {
		b.WriteString("## Failure Logs\n")
		for _, l := range req.FailureLogs {
			fmt.Fprintf(&b, "[%s] %s: %s\n", l.Level, l.Source, l.Message)
		}
		b.WriteString("\n")
	}

	appendPlanDocs(&b, req.PlanDocuments)

	b.WriteString("## Required Response Format\n")
	b.WriteString("SUMMARY: <one-line summary of root cause>\n")
	b.WriteString("FIX_PLAN: <description of the fix>\n")
	b.WriteString("FILES:\n<path>\n---\n<file content>\n---\n")
	b.WriteString("(Repeat FILES block for each file to change. Omit if no file changes needed.)\n")
	return b.String()
}

// buildIntentPrompt constructs the prompt for an intent validation check. Pure function.
func buildIntentPrompt(req ai.IntentValidationRequest) string {
	var b strings.Builder
	b.WriteString("You are GORT, a GitOps reconciliation assistant. A Flux deployment succeeded but ")
	b.WriteString("you must verify the running environment matches the declared intent in the plan documents.\n\n")

	if req.RuntimeState != nil {
		b.WriteString("## Current Runtime State\n")
		for _, pod := range req.RuntimeState.Pods {
			fmt.Fprintf(&b, "Pod %s/%s: phase=%s ready=%v\n", pod.Namespace, pod.Name, pod.Phase, pod.Ready)
		}
		for _, dep := range req.RuntimeState.Deployments {
			fmt.Fprintf(&b, "Deployment %s/%s: desired=%d ready=%d\n",
				dep.Namespace, dep.Name, dep.DesiredReplicas, dep.ReadyReplicas)
		}
		for _, ev := range req.RuntimeState.Events {
			if ev.Type == "Warning" {
				fmt.Fprintf(&b, "Warning event: %s: %s\n", ev.Reason, ev.Message)
			}
		}
		b.WriteString("\n")
	}

	if req.DeclaredSpec != "" {
		fmt.Fprintf(&b, "## Declared Spec\n%s\n\n", req.DeclaredSpec)
	}

	appendPlanDocs(&b, req.PlanDocuments)

	b.WriteString("## Required Response Format\n")
	b.WriteString("INTENT_MET: true|false\n")
	b.WriteString("ISSUES: <comma-separated list of issues, or 'none'>\n")
	b.WriteString("FIX_PLAN: <description of how to fix, or 'none required'>\n")
	b.WriteString("FILES:\n<path>\n---\n<file content>\n---\n")
	b.WriteString("(Include FILES blocks only if intent is not met and file changes are needed.)\n")
	return b.String()
}

func appendPlanDocs(b *strings.Builder, docs []ai.PlanDocument) {
	if len(docs) == 0 {
		return
	}
	b.WriteString("## Plan Documents\n")
	for _, doc := range docs {
		fmt.Fprintf(b, "### %s\n%s\n\n", doc.Path, doc.Content)
	}
}

// parseAnalysisResponse parses the structured text response from an Analyze call. Pure function.
func parseAnalysisResponse(text string) *ai.AnalysisResult {
	result := &ai.AnalysisResult{}
	lines := strings.Split(text, "\n")
	var inFile bool
	var currentPath string
	var fileContent strings.Builder

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "SUMMARY:"):
			result.Summary = strings.TrimSpace(strings.TrimPrefix(line, "SUMMARY:"))
		case strings.HasPrefix(line, "FIX_PLAN:"):
			result.FixPlan = strings.TrimSpace(strings.TrimPrefix(line, "FIX_PLAN:"))
		case strings.HasPrefix(line, "FILES:"):
			// next lines are file path / content pairs
		case !inFile && currentPath == "" && strings.HasSuffix(line, ".yaml") || strings.HasSuffix(line, ".md"):
			currentPath = strings.TrimSpace(line)
		case line == "---" && currentPath != "" && !inFile:
			inFile = true
			fileContent.Reset()
		case line == "---" && inFile:
			result.Files = append(result.Files, ai.FileProposal{
				Path:    currentPath,
				Content: fileContent.String(),
			})
			inFile = false
			currentPath = ""
		case inFile:
			fileContent.WriteString(line + "\n")
		}
	}
	return result
}

// parseIntentResponse parses the structured text response from a ValidateIntent call. Pure function.
func parseIntentResponse(text string) *ai.IntentValidationResult {
	result := &ai.IntentValidationResult{}
	lines := strings.Split(text, "\n")
	var inFile bool
	var currentPath string
	var fileContent strings.Builder

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "INTENT_MET:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "INTENT_MET:"))
			result.Met = strings.EqualFold(val, "true")
		case strings.HasPrefix(line, "ISSUES:"):
			issues := strings.TrimSpace(strings.TrimPrefix(line, "ISSUES:"))
			if issues != "" && !strings.EqualFold(issues, "none") {
				for _, issue := range strings.Split(issues, ",") {
					if trimmed := strings.TrimSpace(issue); trimmed != "" {
						result.Issues = append(result.Issues, trimmed)
					}
				}
			}
		case strings.HasPrefix(line, "FIX_PLAN:"):
			result.FixPlan = strings.TrimSpace(strings.TrimPrefix(line, "FIX_PLAN:"))
		case !inFile && currentPath == "" && (strings.HasSuffix(line, ".yaml") || strings.HasSuffix(line, ".md")):
			currentPath = strings.TrimSpace(line)
		case line == "---" && currentPath != "" && !inFile:
			inFile = true
			fileContent.Reset()
		case line == "---" && inFile:
			result.Files = append(result.Files, ai.FileProposal{
				Path:    currentPath,
				Content: fileContent.String(),
			})
			inFile = false
			currentPath = ""
		case inFile:
			fileContent.WriteString(line + "\n")
		}
	}
	return result
}

