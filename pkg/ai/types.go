// Package ai defines the interface and types for interacting with an AI analysis service
// (e.g. Anthropic Claude, OpenAI GPT).
package ai

import "github.com/clcollins/gort/pkg/gitops"

// PlanDocument is the contents of a docs/plans/*.md file from the target repository.
type PlanDocument struct {
	Path    string
	Content string
}

// FileProposal is a file the AI proposes to create or modify as part of a fix.
type FileProposal struct {
	Path    string
	Content string
}

// AnalysisRequest is sent to the AI when Flux reconciliation has failed.
type AnalysisRequest struct {
	ReconcileStatus *gitops.ReconciliationStatus
	FailureLogs     []gitops.LogEntry
	Resources       []gitops.ManagedResource
	KustomizeYAML   string
	PlanDocuments   []PlanDocument
}

// AnalysisResult is the AI's response to an AnalysisRequest.
type AnalysisResult struct {
	Summary string
	FixPlan string
	Files   []FileProposal
}

// IntentValidationRequest is sent to the AI after a successful reconciliation to verify
// the running environment matches the declared intent in the plan documents.
type IntentValidationRequest struct {
	RuntimeState  *gitops.RuntimeState
	PlanDocuments []PlanDocument
	DeclaredSpec  string
}

// IntentValidationResult is the AI's response to an IntentValidationRequest.
type IntentValidationResult struct {
	Met     bool
	Issues  []string
	FixPlan string
	Files   []FileProposal
}
