// Package vcs defines the interface and types for interacting with a version control
// system (e.g. GitHub, GitLab).
package vcs

import "time"

// PushEvent represents a push to a repository branch, as parsed from a webhook payload.
type PushEvent struct {
	RepoFullName  string
	Branch        string
	CommitSHA     string
	CommitMessage string
	ChangedFiles  []string
	PushedAt      time.Time
}

// FileChange describes a single file to be written in a commit.
type FileChange struct {
	Path    string
	Content []byte
}

// PullRequestInput is the data required to open a pull request.
type PullRequestInput struct {
	Title string
	Body  string
	Head  string
	Base  string
}

// PullRequest is the result of a successfully opened pull request.
type PullRequest struct {
	Number int
	URL    string
	Title  string
}
