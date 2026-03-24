package github_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clcollins/gort/internal/github"
	"github.com/clcollins/gort/pkg/vcs"
	gogithub "github.com/google/go-github/v71/github"
)

// newTestServer returns an httptest.Server and a GitHub client pointed at it.
func newTestServer(t *testing.T, mux *http.ServeMux) (*httptest.Server, *gogithub.Client) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	hc := &http.Client{}
	gc, err := gogithub.NewClient(hc).WithAuthToken("test-token").WithEnterpriseURLs(srv.URL+"/", srv.URL+"/")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return srv, gc
}

func TestValidateWebhook_Valid(t *testing.T) {
	// ValidateWebhook in the GitHub client delegates to the pure webhook.ValidateHMAC;
	// the GitHub package adds secret lookup and the vcs.Client adapter.
	c := github.NewClient(nil, "webhook-secret")

	import_payload := []byte(`{"ref":"refs/heads/main","after":"abc","repository":{"full_name":"org/repo"},"head_commit":{"message":"x","timestamp":"2026-01-01T00:00:00Z"}}`)
	sig := computeHMAC(t, import_payload, "webhook-secret")

	event, err := c.ValidateWebhook(context.Background(), import_payload, "sha256="+sig)
	if err != nil {
		t.Fatalf("ValidateWebhook: %v", err)
	}
	if event.Branch != "main" {
		t.Errorf("branch: got %q", event.Branch)
	}
}

func TestValidateWebhook_InvalidSig(t *testing.T) {
	c := github.NewClient(nil, "webhook-secret")
	_, err := c.ValidateWebhook(context.Background(), []byte("payload"), "sha256=bad")
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestGetFileContents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/org/repo/contents/docs/plans/0001.md", func(w http.ResponseWriter, r *http.Request) {
		content := map[string]any{
			"type":     "file",
			"encoding": "base64",
			"content":  "IyBQbGFu\n", // base64("# Plan")
		}
		if err := json.NewEncoder(w).Encode(content); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	_, gc := newTestServer(t, mux)
	c := github.NewClient(gc, "secret")

	got, err := c.GetFileContents(context.Background(), "org/repo", "docs/plans/0001.md", "main")
	if err != nil {
		t.Fatalf("GetFileContents: %v", err)
	}
	if string(got) != "# Plan" {
		t.Errorf("content: got %q, want %q", string(got), "# Plan")
	}
}

func TestListDirectory(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/org/repo/contents/docs/plans", func(w http.ResponseWriter, r *http.Request) {
		entries := []map[string]any{
			{"type": "file", "path": "docs/plans/0001.md"},
			{"type": "file", "path": "docs/plans/0002.md"},
			{"type": "dir", "path": "docs/plans/sub"},
		}
		if err := json.NewEncoder(w).Encode(entries); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	_, gc := newTestServer(t, mux)
	c := github.NewClient(gc, "secret")

	paths, err := c.ListDirectory(context.Background(), "org/repo", "docs/plans", "main")
	if err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}
	// Only files, not directories.
	if len(paths) != 2 {
		t.Errorf("got %d paths, want 2", len(paths))
	}
}

func TestCreatePullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/org/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusCreated)
		pr := map[string]any{
			"number":   42,
			"html_url": "https://github.com/org/repo/pull/42",
			"title":    "fix: gort auto-fix",
		}
		if err := json.NewEncoder(w).Encode(pr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	_, gc := newTestServer(t, mux)
	c := github.NewClient(gc, "secret")

	pr, err := c.CreatePullRequest(context.Background(), "org/repo", vcs.PullRequestInput{
		Title: "fix: gort auto-fix",
		Body:  "Auto-generated fix PR",
		Head:  "gort/fix-123",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("pr.Number: got %d, want 42", pr.Number)
	}
}

func computeHMAC(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
