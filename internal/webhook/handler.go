// Package webhook implements the HTTP handler for GitHub push webhooks.
// HMAC validation and payload parsing are pure functions; the handler itself
// dispatches to a caller-supplied callback so it can be tested in isolation.
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/clcollins/gort/internal/metrics"
	"github.com/clcollins/gort/pkg/vcs"
)

// DispatchFunc is called with a parsed push event whenever a valid webhook arrives.
type DispatchFunc func(ctx context.Context, event *vcs.PushEvent)

// ValidateHMAC verifies that signature matches the HMAC-SHA256 of payload using secret.
// signature must be in the form "sha256=<hex>".
// This is a pure function.
func ValidateHMAC(payload []byte, signature, secret string) error {
	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return fmt.Errorf("webhook: signature missing %q prefix", prefix)
	}
	gotHex := strings.TrimPrefix(signature, prefix)
	got, err := hex.DecodeString(gotHex)
	if err != nil {
		return fmt.Errorf("webhook: signature hex decode: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	want := mac.Sum(nil)
	if !hmac.Equal(got, want) {
		return fmt.Errorf("webhook: HMAC mismatch")
	}
	return nil
}

// githubPayload is the subset of a GitHub push event payload GORT needs.
type githubPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit struct {
		Message   string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
		Added     []string  `json:"added"`
		Modified  []string  `json:"modified"`
		Removed   []string  `json:"removed"`
	} `json:"head_commit"`
}

// ParseWebhookPayload parses a raw GitHub push webhook payload into a PushEvent.
// This is a pure function.
func ParseWebhookPayload(payload []byte) (*vcs.PushEvent, error) {
	var p githubPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("webhook: parse payload: %w", err)
	}

	branch := strings.TrimPrefix(p.Ref, "refs/heads/")

	changed := make([]string, 0, len(p.HeadCommit.Added)+len(p.HeadCommit.Modified)+len(p.HeadCommit.Removed))
	changed = append(changed, p.HeadCommit.Added...)
	changed = append(changed, p.HeadCommit.Modified...)
	changed = append(changed, p.HeadCommit.Removed...)

	return &vcs.PushEvent{
		RepoFullName:  p.Repository.FullName,
		Branch:        branch,
		CommitSHA:     p.After,
		CommitMessage: p.HeadCommit.Message,
		ChangedFiles:  changed,
		PushedAt:      p.HeadCommit.Timestamp,
	}, nil
}

// FilterByRepo returns the indices of watchers whose targetRepo matches repo.
// getRepo is a function that returns the targetRepo for watcher index i.
// This is a pure function.
func FilterByRepo(repo string, getRepo func(i int) string, count int) []int {
	var matched []int
	for i := range count {
		if getRepo(i) == repo {
			matched = append(matched, i)
		}
	}
	return matched
}

// handler is the HTTP handler for webhook requests.
type handler struct {
	secret   string
	dispatch DispatchFunc
	appCtx   context.Context
}

// NewHandler returns an http.Handler that validates GitHub webhook requests and
// calls dispatch for each valid push event. appCtx is the application lifecycle
// context, canceled on server shutdown.
func NewHandler(secret string, dispatch DispatchFunc, appCtx context.Context) http.Handler {
	return &handler{secret: secret, dispatch: dispatch, appCtx: appCtx}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		metrics.WebhookRequestsTotal.WithLabelValues("invalid").Inc()
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB limit
	if err != nil {
		metrics.WebhookRequestsTotal.WithLabelValues("error").Inc()
		http.Error(w, "cannot read body", http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	if err := ValidateHMAC(body, sig, h.secret); err != nil {
		metrics.WebhookRequestsTotal.WithLabelValues("invalid").Inc()
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType != "push" {
		// Acknowledge non-push events (e.g. ping) without dispatching.
		metrics.WebhookRequestsTotal.WithLabelValues("success").Inc()
		w.WriteHeader(http.StatusOK)
		return
	}

	event, err := ParseWebhookPayload(body)
	if err != nil {
		metrics.WebhookRequestsTotal.WithLabelValues("error").Inc()
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	// Dispatch is non-blocking from the HTTP handler's perspective; the reconciler
	// runs in its own goroutine. Use the application lifecycle context so background
	// work is canceled on server shutdown but not on HTTP request completion.
	go h.dispatch(h.appCtx, event)

	metrics.WebhookRequestsTotal.WithLabelValues("success").Inc()
	w.WriteHeader(http.StatusOK)
}
