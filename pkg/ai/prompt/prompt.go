// Package prompt contains shared prompt-building and response-parsing logic
// used by all AI provider implementations.
package prompt

import (
	"fmt"
	"strings"

	"github.com/clcollins/gort/pkg/ai"
)

// BuildAnalysisPrompt constructs the prompt for a Flux failure analysis. Pure function.
func BuildAnalysisPrompt(req ai.AnalysisRequest) string {
	var b strings.Builder
	b.WriteString("You are GORT, a GitOps reconciliation assistant. A Flux deployment has failed. ")
	b.WriteString("Analyze the failure and propose a fix.\n\n")

	if req.ReconcileStatus != nil {
		fmt.Fprintf(&b, "## Flux Status\nApp: %s/%s\nReady: %v\nReason: %s\nMessage: %s\n\n",
			req.ReconcileStatus.Namespace, req.ReconcileStatus.Name,
			req.ReconcileStatus.Ready, req.ReconcileStatus.Reason, req.ReconcileStatus.Message)
	}

	if len(req.FailureLogs) > 0 {
		b.WriteString("## Failure Logs\n")
		for _, l := range req.FailureLogs {
			fmt.Fprintf(&b, "[%s] %s: %s\n", l.Level, l.Source, l.Message)
		}
		b.WriteString("\n")
	}

	appendPlanDocs(&b, req.PlanDocuments)

	b.WriteString("## Required Response Format\n")
	b.WriteString("SUMMARY: <one-line summary of root cause>\n")
	b.WriteString("FIX_PLAN: <description of the fix>\n")
	b.WriteString("FILES:\n<path>\n---\n<file content>\n---\n")
	b.WriteString("(Repeat FILES block for each file to change. Omit if no file changes needed.)\n")
	return b.String()
}

// BuildIntentPrompt constructs the prompt for an intent validation check. Pure function.
func BuildIntentPrompt(req ai.IntentValidationRequest) string {
	var b strings.Builder
	b.WriteString("You are GORT, a GitOps reconciliation assistant. A Flux deployment succeeded but ")
	b.WriteString("you must verify the running environment matches the declared intent in the plan documents.\n\n")

	if req.RuntimeState != nil {
		b.WriteString("## Current Runtime State\n")
		for _, pod := range req.RuntimeState.Pods {
			fmt.Fprintf(&b, "Pod %s/%s: phase=%s ready=%v\n", pod.Namespace, pod.Name, pod.Phase, pod.Ready)
		}
		for _, dep := range req.RuntimeState.Deployments {
			fmt.Fprintf(&b, "Deployment %s/%s: desired=%d ready=%d\n",
				dep.Namespace, dep.Name, dep.DesiredReplicas, dep.ReadyReplicas)
		}
		for _, ev := range req.RuntimeState.Events {
			if ev.Type == "Warning" {
				fmt.Fprintf(&b, "Warning event: %s: %s\n", ev.Reason, ev.Message)
			}
		}
		b.WriteString("\n")
	}

	if req.DeclaredSpec != "" {
		fmt.Fprintf(&b, "## Declared Spec\n%s\n\n", req.DeclaredSpec)
	}

	appendPlanDocs(&b, req.PlanDocuments)

	b.WriteString("## Required Response Format\n")
	b.WriteString("INTENT_MET: true|false\n")
	b.WriteString("ISSUES: <comma-separated list of issues, or 'none'>\n")
	b.WriteString("FIX_PLAN: <description of how to fix, or 'none required'>\n")
	b.WriteString("FILES:\n<path>\n---\n<file content>\n---\n")
	b.WriteString("(Include FILES blocks only if intent is not met and file changes are needed.)\n")
	return b.String()
}

func appendPlanDocs(b *strings.Builder, docs []ai.PlanDocument) {
	if len(docs) == 0 {
		return
	}
	b.WriteString("## Plan Documents\n")
	for _, doc := range docs {
		fmt.Fprintf(b, "### %s\n%s\n\n", doc.Path, doc.Content)
	}
}

// ParseAnalysisResponse parses the structured text response from an Analyze call. Pure function.
func ParseAnalysisResponse(text string) *ai.AnalysisResult {
	result := &ai.AnalysisResult{}
	lines := strings.Split(text, "\n")
	var inFilesSection bool
	var inFile bool
	var expectingPath bool
	var currentPath string
	var fileContent strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		// File content accumulation must be checked before headers so that
		// lines like "SUMMARY:" inside file content are preserved verbatim.
		case inFile && inFilesSection && trimmed == "---":
			result.Files = append(result.Files, ai.FileProposal{
				Path:    currentPath,
				Content: fileContent.String(),
			})
			inFile = false
			currentPath = ""
			expectingPath = true
		case inFile:
			fileContent.WriteString(line + "\n")
		case strings.HasPrefix(trimmed, "SUMMARY:"):
			result.Summary = strings.TrimSpace(strings.TrimPrefix(trimmed, "SUMMARY:"))
		case strings.HasPrefix(trimmed, "FIX_PLAN:"):
			result.FixPlan = strings.TrimSpace(strings.TrimPrefix(trimmed, "FIX_PLAN:"))
		case strings.HasPrefix(trimmed, "FILES:"):
			inFilesSection = true
			expectingPath = true
			inFile = false
			currentPath = ""
			fileContent.Reset()
		case inFilesSection && expectingPath:
			if trimmed == "" || trimmed == "---" {
				continue
			}
			currentPath = trimmed
			expectingPath = false
		case inFilesSection && trimmed == "---" && currentPath != "":
			inFile = true
			fileContent.Reset()
		}
	}
	if inFile && currentPath != "" {
		result.Files = append(result.Files, ai.FileProposal{
			Path:    currentPath,
			Content: fileContent.String(),
		})
	}
	return result
}

// ParseIntentResponse parses the structured text response from a ValidateIntent call. Pure function.
func ParseIntentResponse(text string) *ai.IntentValidationResult {
	result := &ai.IntentValidationResult{}
	lines := strings.Split(text, "\n")
	var inFilesSection bool
	var inFile bool
	var expectingPath bool
	var currentPath string
	var fileContent strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		// File content accumulation must be checked before headers so that
		// lines like "INTENT_MET:" inside file content are preserved verbatim.
		case inFile && inFilesSection && trimmed == "---":
			result.Files = append(result.Files, ai.FileProposal{
				Path:    currentPath,
				Content: fileContent.String(),
			})
			inFile = false
			currentPath = ""
			expectingPath = true
		case inFile:
			fileContent.WriteString(line + "\n")
		case strings.HasPrefix(trimmed, "INTENT_MET:"):
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "INTENT_MET:"))
			result.Met = strings.EqualFold(val, "true")
		case strings.HasPrefix(trimmed, "ISSUES:"):
			issues := strings.TrimSpace(strings.TrimPrefix(trimmed, "ISSUES:"))
			if issues != "" && !strings.EqualFold(issues, "none") {
				for _, issue := range strings.Split(issues, ",") {
					if t := strings.TrimSpace(issue); t != "" {
						result.Issues = append(result.Issues, t)
					}
				}
			}
		case strings.HasPrefix(trimmed, "FIX_PLAN:"):
			result.FixPlan = strings.TrimSpace(strings.TrimPrefix(trimmed, "FIX_PLAN:"))
		case strings.HasPrefix(trimmed, "FILES:"):
			inFilesSection = true
			expectingPath = true
			inFile = false
			currentPath = ""
			fileContent.Reset()
		case inFilesSection && expectingPath:
			if trimmed == "" || trimmed == "---" {
				continue
			}
			currentPath = trimmed
			expectingPath = false
		case inFilesSection && trimmed == "---" && currentPath != "":
			inFile = true
			fileContent.Reset()
		}
	}
	if inFile && currentPath != "" {
		result.Files = append(result.Files, ai.FileProposal{
			Path:    currentPath,
			Content: fileContent.String(),
		})
	}
	return result
}
