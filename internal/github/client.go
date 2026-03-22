// Package github implements pkg/vcs.Client for GitHub using the go-github SDK.
// All GitHub API calls go through the injected *gogithub.Client, making this
// package fully testable against an httptest server.
package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/clcollins/gort/internal/metrics"
	"github.com/clcollins/gort/internal/webhook"
	"github.com/clcollins/gort/pkg/vcs"
	gogithub "github.com/google/go-github/v71/github"
)

type client struct {
	gh            *gogithub.Client
	webhookSecret string
}

// NewClient returns a vcs.Client backed by the GitHub API.
// gc may be nil only in tests that bypass API calls (e.g. ValidateWebhook tests).
func NewClient(gc *gogithub.Client, webhookSecret string) vcs.Client {
	return &client{gh: gc, webhookSecret: webhookSecret}
}

// ComputeTestHMAC is exported for use in tests only.
// It computes the hex-encoded HMAC-SHA256 of payload using secret.
func ComputeTestHMAC(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *client) ValidateWebhook(ctx context.Context, payload []byte, signature string) (*vcs.PushEvent, error) {
	if err := webhook.ValidateHMAC(payload, signature, c.webhookSecret); err != nil {
		return nil, fmt.Errorf("github: validate webhook: %w", err)
	}
	return webhook.ParseWebhookPayload(payload)
}

func (c *client) GetFileContents(ctx context.Context, repo, path, ref string) ([]byte, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	opts := &gogithub.RepositoryContentGetOptions{Ref: ref}
	file, _, _, apiErr := c.gh.Repositories.GetContents(ctx, owner, name, path, opts)
	if apiErr != nil {
		metrics.VCSRequestsTotal.WithLabelValues("get_file", "error").Inc()
		return nil, fmt.Errorf("github: get file %s@%s: %w", path, ref, apiErr)
	}
	metrics.VCSRequestsTotal.WithLabelValues("get_file", "success").Inc()

	rawContent, err := file.GetContent()
	if err != nil {
		return nil, fmt.Errorf("github: decode file content %s: %w", path, err)
	}
	return []byte(rawContent), nil
}

func (c *client) ListDirectory(ctx context.Context, repo, path, ref string) ([]string, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	opts := &gogithub.RepositoryContentGetOptions{Ref: ref}
	_, dir, _, apiErr := c.gh.Repositories.GetContents(ctx, owner, name, path, opts)
	if apiErr != nil {
		metrics.VCSRequestsTotal.WithLabelValues("list_dir", "error").Inc()
		return nil, fmt.Errorf("github: list dir %s: %w", path, apiErr)
	}
	metrics.VCSRequestsTotal.WithLabelValues("list_dir", "success").Inc()

	var paths []string
	for _, entry := range dir {
		if entry.GetType() == "file" {
			paths = append(paths, entry.GetPath())
		}
	}
	return paths, nil
}

func (c *client) CreateBranch(ctx context.Context, repo, branch, base string) error {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return err
	}
	// Get the base SHA.
	ref, _, apiErr := c.gh.Git.GetRef(ctx, owner, name, "refs/heads/"+base)
	if apiErr != nil {
		metrics.VCSRequestsTotal.WithLabelValues("create_branch", "error").Inc()
		return fmt.Errorf("github: get ref %s: %w", base, apiErr)
	}
	newRef := &gogithub.Reference{
		Ref:    gogithub.Ptr("refs/heads/" + branch),
		Object: &gogithub.GitObject{SHA: ref.Object.SHA},
	}
	if _, _, apiErr = c.gh.Git.CreateRef(ctx, owner, name, newRef); apiErr != nil {
		metrics.VCSRequestsTotal.WithLabelValues("create_branch", "error").Inc()
		return fmt.Errorf("github: create branch %s: %w", branch, apiErr)
	}
	metrics.VCSRequestsTotal.WithLabelValues("create_branch", "success").Inc()
	return nil
}

func (c *client) CommitFiles(ctx context.Context, repo, branch, message string, files []vcs.FileChange) error {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return err
	}
	// Create each file via the contents API (simple single-file commit per change).
	for _, f := range files {
		opts := &gogithub.RepositoryContentFileOptions{
			Message: gogithub.Ptr(message),
			Content: f.Content,
			Branch:  gogithub.Ptr(branch),
		}
		if _, _, apiErr := c.gh.Repositories.CreateFile(ctx, owner, name, f.Path, opts); apiErr != nil {
			metrics.VCSRequestsTotal.WithLabelValues("commit_file", "error").Inc()
			return fmt.Errorf("github: commit file %s: %w", f.Path, apiErr)
		}
	}
	metrics.VCSRequestsTotal.WithLabelValues("commit_files", "success").Inc()
	return nil
}

func (c *client) CreatePullRequest(ctx context.Context, repo string, pr vcs.PullRequestInput) (*vcs.PullRequest, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	input := &gogithub.NewPullRequest{
		Title: gogithub.Ptr(pr.Title),
		Body:  gogithub.Ptr(pr.Body),
		Head:  gogithub.Ptr(pr.Head),
		Base:  gogithub.Ptr(pr.Base),
	}
	result, _, apiErr := c.gh.PullRequests.Create(ctx, owner, name, input)
	if apiErr != nil {
		metrics.VCSRequestsTotal.WithLabelValues("create_pr", "error").Inc()
		return nil, fmt.Errorf("github: create PR: %w", apiErr)
	}
	metrics.VCSRequestsTotal.WithLabelValues("create_pr", "success").Inc()
	return &vcs.PullRequest{
		Number: result.GetNumber(),
		URL:    result.GetHTMLURL(),
		Title:  result.GetTitle(),
	}, nil
}

// splitRepo splits "owner/repo" into (owner, repo).
func splitRepo(repo string) (string, string, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("github: invalid repo %q, expected owner/name", repo)
	}
	return parts[0], parts[1], nil
}
