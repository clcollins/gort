package webhook_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clcollins/gort/internal/webhook"
	"github.com/clcollins/gort/pkg/vcs"
)

// --- pure function tests ---

func TestValidateHMAC_Valid(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"key":"value"}`)
	sig := "sha256=" + computeHMAC(t, payload, secret)

	if err := webhook.ValidateHMAC(payload, sig, secret); err != nil {
		t.Fatalf("expected valid HMAC, got error: %v", err)
	}
}

func TestValidateHMAC_Invalid(t *testing.T) {
	if err := webhook.ValidateHMAC([]byte("body"), "sha256=badhex", "secret"); err == nil {
		t.Fatal("expected error for invalid HMAC")
	}
}

func TestValidateHMAC_WrongSignature(t *testing.T) {
	payload := []byte("body")
	wrongSig := "sha256=" + hex.EncodeToString([]byte("definitely-wrong"))
	if err := webhook.ValidateHMAC(payload, wrongSig, "secret"); err == nil {
		t.Fatal("expected error for wrong signature")
	}
}

func TestValidateHMAC_MissingPrefix(t *testing.T) {
	payload := []byte("body")
	if err := webhook.ValidateHMAC(payload, "noprefixhere", "secret"); err == nil {
		t.Fatal("expected error for missing sha256= prefix")
	}
}

func TestParseWebhookPayload_PushEvent(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	raw := map[string]any{
		"ref": "refs/heads/main",
		"repository": map[string]any{
			"full_name": "clcollins/cluster-config",
		},
		"after": "abc123",
		"head_commit": map[string]any{
			"message":   "fix: update deployment",
			"timestamp": now.Format(time.RFC3339),
			"added":     []string{"docs/plans/0002-fix.md"},
			"modified":  []string{"kustomize/app.yaml"},
			"removed":   []string{},
		},
	}
	payload, _ := json.Marshal(raw)

	event, err := webhook.ParseWebhookPayload(payload)
	if err != nil {
		t.Fatalf("ParseWebhookPayload: %v", err)
	}
	if event.Branch != "main" {
		t.Errorf("branch: got %q, want %q", event.Branch, "main")
	}
	if event.RepoFullName != "clcollins/cluster-config" {
		t.Errorf("repo: got %q, want %q", event.RepoFullName, "clcollins/cluster-config")
	}
	if event.CommitSHA != "abc123" {
		t.Errorf("sha: got %q, want %q", event.CommitSHA, "abc123")
	}
	if len(event.ChangedFiles) != 2 {
		t.Errorf("changed files: got %d, want 2", len(event.ChangedFiles))
	}
}

func TestParseWebhookPayload_NonMainBranch(t *testing.T) {
	raw := map[string]any{
		"ref":        "refs/heads/feature-branch",
		"repository": map[string]any{"full_name": "clcollins/cluster-config"},
		"after":      "def456",
		"head_commit": map[string]any{
			"message":   "wip",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}
	payload, _ := json.Marshal(raw)
	event, err := webhook.ParseWebhookPayload(payload)
	if err != nil {
		t.Fatalf("ParseWebhookPayload: %v", err)
	}
	if event.Branch != "feature-branch" {
		t.Errorf("branch: got %q", event.Branch)
	}
}

func TestParseWebhookPayload_InvalidJSON(t *testing.T) {
	_, err := webhook.ParseWebhookPayload([]byte("not-json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFilterRelevantWatchers(t *testing.T) {
	event := vcs.PushEvent{
		RepoFullName: "clcollins/cluster-config",
		Branch:       "main",
	}
	type watcherSpec struct {
		targetRepo string
	}
	watchers := []watcherSpec{
		{"clcollins/cluster-config"},
		{"clcollins/other-repo"},
		{"clcollins/cluster-config"},
	}

	// Exercise the pure filter function.
	got := webhook.FilterByRepo(event.RepoFullName, func(i int) string {
		return watchers[i].targetRepo
	}, len(watchers))

	if len(got) != 2 {
		t.Errorf("got %d matches, want 2", len(got))
	}
}

// --- HTTP handler tests ---

func TestHandler_ValidRequest(t *testing.T) {
	secret := "test-secret"
	payload := pushPayload(t)

	dispatched := make(chan *vcs.PushEvent, 1)
	handler := webhook.NewHandler(secret, func(_ context.Context, e *vcs.PushEvent) {
		dispatched <- e
	}, context.Background())

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+computeHMAC(t, payload, secret))
	req.Header.Set("X-GitHub-Event", "push")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}

	// The handler dispatches in a goroutine; wait for it with a timeout.
	select {
	case captured := <-dispatched:
		if captured == nil {
			t.Fatal("dispatch received nil event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch callback was not called within timeout")
	}
}

func TestHandler_InvalidHMAC(t *testing.T) {
	handler := webhook.NewHandler("secret", func(_ context.Context, _ *vcs.PushEvent) {}, context.Background())
	payload := pushPayload(t)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256=badhex")
	req.Header.Set("X-GitHub-Event", "push")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}

func TestHandler_NonPushEvent(t *testing.T) {
	secret := "secret"
	payload := pushPayload(t)
	called := false
	handler := webhook.NewHandler(secret, func(_ context.Context, _ *vcs.PushEvent) {
		called = true
	}, context.Background())

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("X-Hub-Signature-256", "sha256="+computeHMAC(t, payload, secret))
	req.Header.Set("X-GitHub-Event", "ping")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200 for ping", rr.Code)
	}
	if called {
		t.Error("dispatch callback should not be called for ping events")
	}
}

func TestHandler_WrongMethod(t *testing.T) {
	handler := webhook.NewHandler("secret", func(_ context.Context, _ *vcs.PushEvent) {}, context.Background())
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want 405", rr.Code)
	}
}

// helpers

func computeHMAC(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func pushPayload(t *testing.T) []byte {
	t.Helper()
	raw := map[string]any{
		"ref": "refs/heads/main",
		"repository": map[string]any{
			"full_name": "clcollins/cluster-config",
		},
		"after": "abc123",
		"head_commit": map[string]any{
			"message":   fmt.Sprintf("fix: update at %d", time.Now().Unix()),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"added":     []string{},
			"modified":  []string{"kustomize/app.yaml"},
			"removed":   []string{},
		},
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}
