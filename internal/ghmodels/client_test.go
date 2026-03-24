package ghmodels_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clcollins/gort/internal/ghmodels"
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
		// Verify required headers.
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("expected Authorization 'Bearer test-token', got %q", auth)
		}
		if accept := r.Header.Get("Accept"); accept != "application/vnd.github+json" {
			t.Errorf("expected Accept 'application/vnd.github+json', got %q", accept)
		}
		if apiVer := r.Header.Get("X-GitHub-Api-Version"); apiVer == "" {
			t.Error("expected X-GitHub-Api-Version header to be set")
		}
		// Verify endpoint path.
		if r.URL.Path != "/inference/chat/completions" {
			t.Errorf("expected path /inference/chat/completions, got %q", r.URL.Path)
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
	c := ghmodels.NewClient("test-token", "openai/gpt-4.1", ghmodels.WithBaseURL(srv.URL))

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "authentication_error", "message": "Invalid token"}}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)

	c := ghmodels.NewClient("bad-token", "openai/gpt-4.1", ghmodels.WithBaseURL(srv.URL))
	_, err := c.Analyze(context.Background(), ai.AnalysisRequest{})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestValidateIntent_Met(t *testing.T) {
	responseText := `INTENT_MET: true
ISSUES: none
FIX_PLAN: none required`

	srv := newTestServer(t, chatCompletionResponse(responseText))
	c := ghmodels.NewClient("test-token", "openai/gpt-4.1", ghmodels.WithBaseURL(srv.URL))

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
	c := ghmodels.NewClient("test-token", "openai/gpt-4.1", ghmodels.WithBaseURL(srv.URL))

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
