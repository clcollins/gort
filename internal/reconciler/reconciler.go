// Package reconciler implements the core GORT reconciliation loop.
// It is the only non-pure package in internal/: it orchestrates calls to the
// gitops, vcs, and ai interfaces to produce fix PRs.
package reconciler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/clcollins/gort/internal/metrics"
	"github.com/clcollins/gort/pkg/ai"
	"github.com/clcollins/gort/pkg/gitops"
	"github.com/clcollins/gort/pkg/vcs"
)

// Input holds the parameters for a single reconciliation run.
type Input struct {
	WatcherName string
	Namespace   string
	TargetRepo  string
	FixRepo     string
	DocsPaths   []string
	Timeout     time.Duration
}

// Reconciler orchestrates the full GORT reconcile flow.
type Reconciler interface {
	// Reconcile runs one reconcile cycle for the given input.
	// It returns the fix PR if one was opened, nil if no action was needed.
	Reconcile(ctx context.Context, in Input) (*vcs.PullRequest, error)
}

type reconciler struct {
	gitops gitops.Client
	vcs    vcs.Client
	ai     ai.Client
}

// New constructs a Reconciler with injected interface dependencies.
func New(g gitops.Client, v vcs.Client, a ai.Client) Reconciler {
	return &reconciler{gitops: g, vcs: v, ai: a}
}

// Reconcile is the only non-pure function in GORT (besides external API callers).
// It runs the full reconciliation flow:
//  1. Watch Flux until reconciliation completes or fails.
//  2. If failure: ask AI to analyze → open fix PR.
//  3. If success: collect runtime state → ask AI to validate intent.
//     If intent not met: open fix PR.
func (r *reconciler) Reconcile(ctx context.Context, in Input) (*vcs.PullRequest, error) {
	timeout := in.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	start := time.Now()
	result, err := r.gitops.WatchReconciliation(ctx, in.WatcherName, in.Namespace, timeout)
	metrics.ReconcileDurationSeconds.WithLabelValues(in.WatcherName).Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.ReconcilePollsTotal.WithLabelValues(in.WatcherName, "timeout").Inc()
		return nil, fmt.Errorf("reconciler: watch %s: %w", in.WatcherName, err)
	}

	planDocs, err := r.fetchPlanDocuments(ctx, in.TargetRepo, in.DocsPaths)
	if err != nil {
		// Non-fatal: proceed without plan docs.
		planDocs = nil
	}

	if !result.Succeeded {
		metrics.ReconcilePollsTotal.WithLabelValues(in.WatcherName, "failure").Inc()
		pr, err := r.handleFailure(ctx, in, result, planDocs)
		if err != nil {
			metrics.FixPRsFailedTotal.WithLabelValues(in.WatcherName, "flux_failure").Inc()
			return nil, err
		}
		metrics.FixPRsOpenedTotal.WithLabelValues(in.WatcherName, "flux_failure").Inc()
		return pr, nil
	}

	metrics.ReconcilePollsTotal.WithLabelValues(in.WatcherName, "success").Inc()

	// Flux succeeded — validate the running environment matches intent.
	runtimeState, err := r.gitops.GetRuntimeState(ctx, in.WatcherName, in.Namespace)
	if err != nil {
		// Non-fatal: skip intent validation.
		return nil, nil
	}

	intentResult, err := r.ai.ValidateIntent(ctx, ai.IntentValidationRequest{
		RuntimeState:  runtimeState,
		PlanDocuments: planDocs,
	})
	if err != nil {
		metrics.IntentValidationTotal.WithLabelValues(in.WatcherName, "error").Inc()
		return nil, fmt.Errorf("reconciler: validate intent: %w", err)
	}

	if intentResult.Met {
		metrics.IntentValidationTotal.WithLabelValues(in.WatcherName, "met").Inc()
		return nil, nil
	}

	metrics.IntentValidationTotal.WithLabelValues(in.WatcherName, "not_met").Inc()
	pr, err := r.openFixPR(ctx, in, "intent_not_met", intentSummary(intentResult), intentResult.FixPlan, intentResult.Files)
	if err != nil {
		metrics.FixPRsFailedTotal.WithLabelValues(in.WatcherName, "intent_not_met").Inc()
		return nil, err
	}
	metrics.FixPRsOpenedTotal.WithLabelValues(in.WatcherName, "intent_not_met").Inc()
	return pr, nil
}

func (r *reconciler) handleFailure(ctx context.Context, in Input, result *gitops.ReconciliationResult, planDocs []ai.PlanDocument) (*vcs.PullRequest, error) {
	analysis, err := r.ai.Analyze(ctx, ai.AnalysisRequest{
		ReconcileStatus: &result.Status,
		FailureLogs:     result.Logs,
		Resources:       result.Resources,
		PlanDocuments:   planDocs,
	})
	if err != nil {
		return nil, fmt.Errorf("reconciler: AI analyze: %w", err)
	}

	return r.openFixPR(ctx, in, "flux_failure", analysis.Summary, analysis.FixPlan, analysis.Files)
}

func (r *reconciler) openFixPR(ctx context.Context, in Input, reason, summary, fixPlan string, files []ai.FileProposal) (*vcs.PullRequest, error) {
	if in.FixRepo == "" {
		return nil, fmt.Errorf("reconciler: fixRepo is not set")
	}

	branch := BuildBranchName(reason, in.WatcherName)

	// Get the base SHA from main.
	if err := r.vcs.CreateBranch(ctx, in.FixRepo, branch, "main"); err != nil {
		return nil, fmt.Errorf("reconciler: create branch %s: %w", branch, err)
	}

	// Build file changes: proposed fixes + a plan doc.
	fileChanges := make([]vcs.FileChange, 0, len(files)+1)
	for _, f := range files {
		fileChanges = append(fileChanges, vcs.FileChange{
			Path:    f.Path,
			Content: []byte(f.Content),
		})
	}
	// Add a GORT plan document to the fix PR.
	planPath := fmt.Sprintf("docs/plans/gort-fix-%d.md", time.Now().UnixNano())
	fileChanges = append(fileChanges, vcs.FileChange{
		Path:    planPath,
		Content: []byte(buildGORTPlanDoc(reason, summary, fixPlan)),
	})

	commitMsg := fmt.Sprintf("fix(%s): %s", in.WatcherName, summary)
	if err := r.vcs.CommitFiles(ctx, in.FixRepo, branch, commitMsg, fileChanges); err != nil {
		return nil, fmt.Errorf("reconciler: commit files: %w", err)
	}

	prTitle := fmt.Sprintf("fix(%s): %s", in.WatcherName, truncate(summary, 60))
	pr, err := r.vcs.CreatePullRequest(ctx, in.FixRepo, vcs.PullRequestInput{
		Title: prTitle,
		Body:  BuildPRBody(reason, in.WatcherName, summary, fixPlan),
		Head:  branch,
		Base:  "main",
	})
	if err != nil {
		return nil, fmt.Errorf("reconciler: create PR: %w", err)
	}
	return pr, nil
}

// fetchPlanDocuments reads all markdown files from the configured docs paths in the repo.
func (r *reconciler) fetchPlanDocuments(ctx context.Context, repo string, docsPaths []string) ([]ai.PlanDocument, error) {
	if repo == "" {
		return nil, nil
	}
	paths := docsPaths
	if len(paths) == 0 {
		paths = []string{"docs/plans/"}
	}

	var docs []ai.PlanDocument
	for _, dir := range paths {
		entries, err := r.vcs.ListDirectory(ctx, repo, strings.TrimRight(dir, "/"), "main")
		if err != nil {
			continue
		}
		for _, path := range entries {
			if !strings.HasSuffix(path, ".md") {
				continue
			}
			content, err := r.vcs.GetFileContents(ctx, repo, path, "main")
			if err != nil {
				continue
			}
			docs = append(docs, ai.PlanDocument{Path: path, Content: string(content)})
		}
	}
	return docs, nil
}

// BuildBranchName creates a URL-safe branch name with a nanosecond timestamp.
func BuildBranchName(reason, watcherName string) string {
	ts := time.Now().UTC().Format("20060102-150405.000000000")
	name := fmt.Sprintf("gort/%s/%s/%s", reason, watcherName, ts)
	// Replace characters unsafe in git branch names.
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

// BuildPRBody constructs the pull request body markdown. Pure function.
func BuildPRBody(reason, watcherName, summary, fixPlan string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## GORT Auto-Fix: %s\n\n", watcherName)
	fmt.Fprintf(&b, "**Reason:** `%s`\n\n", reason)
	fmt.Fprintf(&b, "### Summary\n%s\n\n", summary)
	fmt.Fprintf(&b, "### Proposed Fix\n%s\n\n", fixPlan)
	b.WriteString("---\n*This PR was automatically generated by [GORT](https://github.com/clcollins/gort).*\n")
	return b.String()
}

// buildGORTPlanDoc generates the docs/plans markdown file for a GORT-opened PR. Pure function.
func buildGORTPlanDoc(reason, summary, fixPlan string) string {
	return fmt.Sprintf("# GORT Auto-Fix Plan\n\n**Reason:** %s\n\n## Summary\n%s\n\n## Fix Plan\n%s\n",
		reason, summary, fixPlan)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// intentSummary returns a single-line description of the intent validation result.
// Pure function.
func intentSummary(r *ai.IntentValidationResult) string {
	if len(r.Issues) == 0 {
		return "intent not met"
	}
	return strings.Join(r.Issues, "; ")
}
