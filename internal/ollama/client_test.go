package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clcollins/gort/internal/ollama"
	"github.com/clcollins/gort/pkg/ai"
	"github.com/clcollins/gort/pkg/gitops"
)

// chatCompletionResponse returns a minimal OpenAI-compatible chat completions response.
func chatCompletionResponse(content string) map[string]any {
	return map[string]any{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
	}
}

func newTestServer(t *testing.T, response map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify endpoint path.
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %q", r.URL.Path)
		}
		// Verify HTTP method.
		if r.Method != http.MethodPost {
			t.Errorf("expected method POST, got %q", r.Method)
		}
		// Verify no Authorization header (Ollama doesn't require auth).
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %q", auth)
		}
		// Verify request body contains expected JSON structure.
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request body as JSON: %v", err)
		}
		if model, ok := payload["model"].(string); !ok || model == "" {
			t.Errorf("expected non-empty 'model' field, got %v", payload["model"])
		}
		messages, ok := payload["messages"].([]any)
		if !ok || len(messages) == 0 {
			t.Errorf("expected non-empty 'messages' array, got %v", payload["messages"])
		}
		if _, ok := payload["max_tokens"]; !ok {
			t.Error("expected 'max_tokens' field in request body")
		}
		// Verify stream is explicitly false.
		if stream, ok := payload["stream"].(bool); !ok || stream {
			t.Errorf("expected 'stream' to be false, got %v", payload["stream"])
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnalyze_Success(t *testing.T) {
	responseText := `SUMMARY: kustomize build failed due to missing resource
FIX_PLAN: Update the kustomize overlay to reference the correct base path
FILES:
kustomize/overlay/kustomization.yaml
---
resources:
- ../../base
---`

	srv := newTestServer(t, chatCompletionResponse(responseText))
	c := ollama.NewClient("llama3", ollama.WithBaseURL(srv.URL))

	req := ai.AnalysisRequest{
		ReconcileStatus: &gitops.ReconciliationStatus{
			Name:      "cluster-config",
			Namespace: "flux-system",
			Ready:     false,
			Reason:    "BuildFailed",
			Message:   "kustomize build failed: no such file",
		},
		FailureLogs: []gitops.LogEntry{
			{Level: "error", Message: "kustomize build failed", Source: "kustomize-controller"},
		},
		PlanDocuments: []ai.PlanDocument{
			{Path: "docs/plans/0001.md", Content: "# Plan\nDeploy app to cluster"},
		},
	}

	result, err := c.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Summary == "" {
		t.Error("expected non-empty Summary")
	}
}

func TestAnalyze_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "server_error", "message": "model not found"}}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	c := ollama.NewClient("nonexistent-model", ollama.WithBaseURL(srv.URL))
	_, err := c.Analyze(context.Background(), ai.AnalysisRequest{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestValidateIntent_Met(t *testing.T) {
	responseText := `INTENT_MET: true
ISSUES: none
FIX_PLAN: none required`

	srv := newTestServer(t, chatCompletionResponse(responseText))
	c := ollama.NewClient("llama3", ollama.WithBaseURL(srv.URL))

	req := ai.IntentValidationRequest{
		RuntimeState: &gitops.RuntimeState{
			Pods: []gitops.PodState{
				{Name: "app-pod", Namespace: "default", Ready: true, Phase: "Running"},
			},
		},
		PlanDocuments: []ai.PlanDocument{
			{Path: "docs/plans/0001.md", Content: "# Plan\nRun one pod"},
		},
		DeclaredSpec: "replicas: 1",
	}

	result, err := c.ValidateIntent(context.Background(), req)
	if err != nil {
		t.Fatalf("ValidateIntent: %v", err)
	}
	if !result.Met {
		t.Errorf("expected Met=true, issues: %v", result.Issues)
	}
}

func TestValidateIntent_NotMet(t *testing.T) {
	responseText := `INTENT_MET: false
ISSUES: Only 0 of 3 replicas are ready
FIX_PLAN: Increase resource limits in the deployment
FILES:
kustomize/app/deployment.yaml
---
resources:
  limits:
    memory: 256Mi
---`

	srv := newTestServer(t, chatCompletionResponse(responseText))
	c := ollama.NewClient("llama3", ollama.WithBaseURL(srv.URL))

	req := ai.IntentValidationRequest{
		RuntimeState: &gitops.RuntimeState{
			Pods: []gitops.PodState{
				{Name: "app-pod-0", Ready: false, Phase: "OOMKilled"},
			},
		},
		PlanDocuments: []ai.PlanDocument{
			{Path: "docs/plans/0001.md", Content: "# Plan\nRun 3 replicas"},
		},
	}

	result, err := c.ValidateIntent(context.Background(), req)
	if err != nil {
		t.Fatalf("ValidateIntent: %v", err)
	}
	if result.Met {
		t.Error("expected Met=false for not-ready pods")
	}
	if len(result.Issues) == 0 {
		t.Error("expected non-empty Issues")
	}
}
