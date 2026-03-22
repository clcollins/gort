package reconciler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/clcollins/gort/internal/reconciler"
	"github.com/clcollins/gort/pkg/ai"
	"github.com/clcollins/gort/pkg/gitops"
	"github.com/clcollins/gort/pkg/vcs"
)

// --- mock implementations ---

type mockGitOps struct {
	status    *gitops.ReconciliationStatus
	result    *gitops.ReconciliationResult
	runtime   *gitops.RuntimeState
	statusErr error
	watchErr  error
}

func (m *mockGitOps) GetReconciliationStatus(_ context.Context, _, _ string) (*gitops.ReconciliationStatus, error) {
	return m.status, m.statusErr
}
func (m *mockGitOps) GetFailureLogs(_ context.Context, _, _ string) ([]gitops.LogEntry, error) {
	return nil, nil
}
func (m *mockGitOps) GetManagedResources(_ context.Context, _, _ string) ([]gitops.ManagedResource, error) {
	return nil, nil
}
func (m *mockGitOps) WatchReconciliation(_ context.Context, _, _ string, _ time.Duration) (*gitops.ReconciliationResult, error) {
	return m.result, m.watchErr
}
func (m *mockGitOps) GetRuntimeState(_ context.Context, _, _ string) (*gitops.RuntimeState, error) {
	return m.runtime, nil
}

type mockVCS struct {
	planFiles  map[string][]byte
	dirEntries []string
	pr         *vcs.PullRequest
	prErr      error
	branchErr  error
	commitErr  error
}

func (m *mockVCS) ValidateWebhook(_ context.Context, _ []byte, _ string) (*vcs.PushEvent, error) {
	return nil, nil
}
func (m *mockVCS) GetFileContents(_ context.Context, _, path, _ string) ([]byte, error) {
	if m.planFiles != nil {
		if content, ok := m.planFiles[path]; ok {
			return content, nil
		}
	}
	return nil, nil
}
func (m *mockVCS) ListDirectory(_ context.Context, _, _, _ string) ([]string, error) {
	return m.dirEntries, nil
}
func (m *mockVCS) CreateBranch(_ context.Context, _, _, _ string) error {
	return m.branchErr
}
func (m *mockVCS) CommitFiles(_ context.Context, _, _, _ string, _ []vcs.FileChange) error {
	return m.commitErr
}
func (m *mockVCS) CreatePullRequest(_ context.Context, _ string, _ vcs.PullRequestInput) (*vcs.PullRequest, error) {
	return m.pr, m.prErr
}

type mockAI struct {
	analysisResult *ai.AnalysisResult
	intentResult   *ai.IntentValidationResult
	analysisErr    error
	intentErr      error
}

func (m *mockAI) Analyze(_ context.Context, _ ai.AnalysisRequest) (*ai.AnalysisResult, error) {
	return m.analysisResult, m.analysisErr
}
func (m *mockAI) ValidateIntent(_ context.Context, _ ai.IntentValidationRequest) (*ai.IntentValidationResult, error) {
	return m.intentResult, m.intentErr
}

// --- pure function tests ---

func TestBuildBranchName(t *testing.T) {
	name := reconciler.BuildBranchName("flux_failure", "cluster-config")
	if name == "" {
		t.Fatal("expected non-empty branch name")
	}
	if len(name) > 100 {
		t.Errorf("branch name too long: %d chars", len(name))
	}
}

func TestBuildPRBody_FluxFailure(t *testing.T) {
	result := &ai.AnalysisResult{
		Summary: "kustomize build failed",
		FixPlan: "Fix the overlay path",
	}
	body := reconciler.BuildPRBody("flux_failure", "cluster-config", result.Summary, result.FixPlan)
	if body == "" {
		t.Fatal("expected non-empty PR body")
	}
}

func TestBuildPRBody_IntentNotMet(t *testing.T) {
	body := reconciler.BuildPRBody("intent_not_met", "cluster-config", "replica count mismatch", "Increase replicas")
	if body == "" {
		t.Fatal("expected non-empty PR body")
	}
}

// --- reconcile flow tests ---

func TestReconcile_FluxFailure_OpensPR(t *testing.T) {
	gitopsClient := &mockGitOps{
		result: &gitops.ReconciliationResult{
			Succeeded: false,
			Status: gitops.ReconciliationStatus{
				Name:      "cluster-config",
				Namespace: "flux-system",
				Ready:     false,
				Reason:    "BuildFailed",
				Message:   "kustomize build failed",
			},
		},
	}
	aiClient := &mockAI{
		analysisResult: &ai.AnalysisResult{
			Summary: "build failed",
			FixPlan: "fix the overlay",
			Files:   []ai.FileProposal{{Path: "kustomize/app.yaml", Content: "fixed: true\n"}},
		},
	}
	vcsClient := &mockVCS{
		dirEntries: []string{"docs/plans/0001.md"},
		planFiles:  map[string][]byte{"docs/plans/0001.md": []byte("# Plan")},
		pr:         &vcs.PullRequest{Number: 1, URL: "https://github.com/org/repo/pull/1"},
	}

	r := reconciler.New(gitopsClient, vcsClient, aiClient)
	pr, err := r.Reconcile(context.Background(), reconciler.Input{
		WatcherName: "cluster-config",
		Namespace:   "flux-system",
		TargetRepo:  "org/repo",
		FixRepo:     "org/repo",
		DocsPaths:   []string{"docs/plans/"},
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if pr == nil {
		t.Fatal("expected fix PR to be opened")
	}
	if pr.Number != 1 {
		t.Errorf("pr.Number: got %d, want 1", pr.Number)
	}
}

func TestReconcile_FluxSuccess_IntentMet_NoPR(t *testing.T) {
	gitopsClient := &mockGitOps{
		result: &gitops.ReconciliationResult{
			Succeeded: true,
			Status:    gitops.ReconciliationStatus{Ready: true, Reason: "ReconciliationSucceeded"},
		},
		runtime: &gitops.RuntimeState{
			Pods: []gitops.PodState{{Name: "app", Ready: true, Phase: "Running"}},
		},
	}
	aiClient := &mockAI{
		intentResult: &ai.IntentValidationResult{Met: true},
	}
	vcsClient := &mockVCS{
		dirEntries: []string{"docs/plans/0001.md"},
		planFiles:  map[string][]byte{"docs/plans/0001.md": []byte("# Plan")},
	}

	r := reconciler.New(gitopsClient, vcsClient, aiClient)
	pr, err := r.Reconcile(context.Background(), reconciler.Input{
		WatcherName: "cluster-config",
		Namespace:   "flux-system",
		TargetRepo:  "org/repo",
		FixRepo:     "org/repo",
		DocsPaths:   []string{"docs/plans/"},
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if pr != nil {
		t.Errorf("expected no fix PR when intent is met, got: %+v", pr)
	}
}

func TestReconcile_FluxSuccess_IntentNotMet_OpensPR(t *testing.T) {
	gitopsClient := &mockGitOps{
		result: &gitops.ReconciliationResult{
			Succeeded: true,
			Status:    gitops.ReconciliationStatus{Ready: true, Reason: "ReconciliationSucceeded"},
		},
		runtime: &gitops.RuntimeState{
			Pods: []gitops.PodState{{Name: "app", Ready: false, Phase: "OOMKilled"}},
		},
	}
	aiClient := &mockAI{
		intentResult: &ai.IntentValidationResult{
			Met:     false,
			Issues:  []string{"pods not ready"},
			FixPlan: "increase memory limits",
			Files:   []ai.FileProposal{{Path: "kustomize/app.yaml", Content: "memory: 256Mi\n"}},
		},
	}
	vcsClient := &mockVCS{
		dirEntries: []string{"docs/plans/0001.md"},
		planFiles:  map[string][]byte{"docs/plans/0001.md": []byte("# Plan")},
		pr:         &vcs.PullRequest{Number: 2, URL: "https://github.com/org/repo/pull/2"},
	}

	r := reconciler.New(gitopsClient, vcsClient, aiClient)
	pr, err := r.Reconcile(context.Background(), reconciler.Input{
		WatcherName: "cluster-config",
		Namespace:   "flux-system",
		TargetRepo:  "org/repo",
		FixRepo:     "org/repo",
		DocsPaths:   []string{"docs/plans/"},
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if pr == nil {
		t.Fatal("expected fix PR when intent is not met")
	}
	if pr.Number != 2 {
		t.Errorf("pr.Number: got %d, want 2", pr.Number)
	}
}

func TestReconcile_WatchError_ReturnsError(t *testing.T) {
	gitopsClient := &mockGitOps{
		watchErr: errors.New("timeout watching reconciliation"),
	}
	r := reconciler.New(gitopsClient, &mockVCS{}, &mockAI{})
	_, err := r.Reconcile(context.Background(), reconciler.Input{
		WatcherName: "cluster-config",
		Namespace:   "flux-system",
		Timeout:     1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error when watch fails")
	}
}

func TestReconcile_PRCreateError_ReturnsError(t *testing.T) {
	gitopsClient := &mockGitOps{
		result: &gitops.ReconciliationResult{
			Succeeded: false,
			Status:    gitops.ReconciliationStatus{Ready: false, Reason: "BuildFailed"},
		},
	}
	aiClient := &mockAI{
		analysisResult: &ai.AnalysisResult{Summary: "failed", FixPlan: "fix it"},
	}
	vcsClient := &mockVCS{
		prErr: errors.New("GitHub API 422"),
	}

	r := reconciler.New(gitopsClient, vcsClient, aiClient)
	_, err := r.Reconcile(context.Background(), reconciler.Input{
		WatcherName: "cluster-config",
		Namespace:   "flux-system",
		FixRepo:     "org/repo",
		Timeout:     1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error when PR creation fails")
	}
}
