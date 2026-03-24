package vcs

import "context"

// Client is the interface for interacting with a VCS hosting service.
// Implementations cover GitHub, GitLab, Gitea, etc.
// All implementations must be safe for concurrent use.
type Client interface {
	// ValidateWebhook verifies the HMAC signature and returns the parsed push event.
	ValidateWebhook(ctx context.Context, payload []byte, signature string) (*PushEvent, error)

	// GetFileContents returns the raw bytes of a file at the given path and git ref.
	GetFileContents(ctx context.Context, repo, path, ref string) ([]byte, error)

	// ListDirectory returns the file paths within a directory at the given git ref.
	ListDirectory(ctx context.Context, repo, path, ref string) ([]string, error)

	// CreateBranch creates a new branch from the given base ref.
	CreateBranch(ctx context.Context, repo, branch, base string) error

	// CommitFiles creates a single commit on the given branch containing all file changes.
	CommitFiles(ctx context.Context, repo, branch, message string, files []FileChange) error

	// CreatePullRequest opens a pull request and returns the result.
	CreatePullRequest(ctx context.Context, repo string, pr PullRequestInput) (*PullRequest, error)
}
