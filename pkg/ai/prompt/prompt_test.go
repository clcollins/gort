package prompt_test

import (
	"strings"
	"testing"

	"github.com/clcollins/gort/pkg/ai"
	"github.com/clcollins/gort/pkg/ai/prompt"
	"github.com/clcollins/gort/pkg/gitops"
)

func TestBuildAnalysisPrompt_ContainsSections(t *testing.T) {
	req := ai.AnalysisRequest{
		ReconcileStatus: &gitops.ReconciliationStatus{
			Name:      "cluster-config",
			Namespace: "flux-system",
			Ready:     false,
			Reason:    "BuildFailed",
			Message:   "kustomize build failed",
		},
		FailureLogs: []gitops.LogEntry{
			{Level: "error", Source: "kustomize-controller", Message: "build failed"},
		},
		PlanDocuments: []ai.PlanDocument{
			{Path: "docs/plans/0001.md", Content: "# Plan\nDeploy app"},
		},
	}

	p := prompt.BuildAnalysisPrompt(req)

	for _, want := range []string{
		"## Flux Status",
		"flux-system/cluster-config",
		"## Failure Logs",
		"[error] kustomize-controller: build failed",
		"## Plan Documents",
		"docs/plans/0001.md",
		"## Required Response Format",
		"SUMMARY:",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildIntentPrompt_ContainsSections(t *testing.T) {
	req := ai.IntentValidationRequest{
		RuntimeState: &gitops.RuntimeState{
			Pods: []gitops.PodState{
				{Name: "app-pod", Namespace: "default", Ready: true, Phase: "Running"},
			},
			Deployments: []gitops.DeploymentState{
				{Name: "app", Namespace: "default", DesiredReplicas: 3, ReadyReplicas: 3},
			},
		},
		PlanDocuments: []ai.PlanDocument{
			{Path: "docs/plans/0001.md", Content: "# Plan\nRun 3 replicas"},
		},
		DeclaredSpec: "replicas: 3",
	}

	p := prompt.BuildIntentPrompt(req)

	for _, want := range []string{
		"## Current Runtime State",
		"Pod default/app-pod",
		"Deployment default/app",
		"## Declared Spec",
		"replicas: 3",
		"## Plan Documents",
		"INTENT_MET:",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestParseAnalysisResponse(t *testing.T) {
	text := `SUMMARY: kustomize build failed due to missing resource
FIX_PLAN: Update the kustomize overlay
FILES:
kustomize/overlay/kustomization.yaml
---
resources:
- ../../base
---`

	result := prompt.ParseAnalysisResponse(text)

	if result.Summary != "kustomize build failed due to missing resource" {
		t.Errorf("Summary = %q", result.Summary)
	}
	if result.FixPlan != "Update the kustomize overlay" {
		t.Errorf("FixPlan = %q", result.FixPlan)
	}
	if len(result.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(result.Files))
	}
	if result.Files[0].Path != "kustomize/overlay/kustomization.yaml" {
		t.Errorf("Files[0].Path = %q", result.Files[0].Path)
	}
}

func TestParseAnalysisResponse_NoFiles(t *testing.T) {
	text := `SUMMARY: timeout waiting for health check
FIX_PLAN: Increase the health check timeout`

	result := prompt.ParseAnalysisResponse(text)

	if result.Summary != "timeout waiting for health check" {
		t.Errorf("Summary = %q", result.Summary)
	}
	if len(result.Files) != 0 {
		t.Errorf("len(Files) = %d, want 0", len(result.Files))
	}
}

func TestParseIntentResponse_Met(t *testing.T) {
	text := `INTENT_MET: true
ISSUES: none
FIX_PLAN: none required`

	result := prompt.ParseIntentResponse(text)

	if !result.Met {
		t.Error("expected Met=true")
	}
	if len(result.Issues) != 0 {
		t.Errorf("Issues = %v, want empty", result.Issues)
	}
}

func TestParseIntentResponse_NotMet(t *testing.T) {
	text := `INTENT_MET: false
ISSUES: Only 0 of 3 replicas are ready, OOMKilled detected
FIX_PLAN: Increase resource limits
FILES:
kustomize/app/deployment.yaml
---
resources:
  limits:
    memory: 256Mi
---`

	result := prompt.ParseIntentResponse(text)

	if result.Met {
		t.Error("expected Met=false")
	}
	if len(result.Issues) != 2 {
		t.Fatalf("len(Issues) = %d, want 2", len(result.Issues))
	}
	if result.Issues[0] != "Only 0 of 3 replicas are ready" {
		t.Errorf("Issues[0] = %q", result.Issues[0])
	}
	if len(result.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(result.Files))
	}
}

func TestParseAnalysisResponse_NonYAMLExtension(t *testing.T) {
	text := `SUMMARY: terraform config needs update
FIX_PLAN: Update variables
FILES:
infra/main.tf
---
resource "aws_instance" "web" {}
---`

	result := prompt.ParseAnalysisResponse(text)

	if len(result.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(result.Files))
	}
	if result.Files[0].Path != "infra/main.tf" {
		t.Errorf("Files[0].Path = %q, want %q", result.Files[0].Path, "infra/main.tf")
	}
}

func TestParseAnalysisResponse_MissingTrailingDelimiter(t *testing.T) {
	text := `SUMMARY: missing config
FIX_PLAN: Add config file
FILES:
config/settings.json
---
{"debug": true}`

	result := prompt.ParseAnalysisResponse(text)

	if len(result.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(result.Files))
	}
	if result.Files[0].Path != "config/settings.json" {
		t.Errorf("Files[0].Path = %q", result.Files[0].Path)
	}
}

func TestParseIntentResponse_NonYAMLExtension(t *testing.T) {
	text := `INTENT_MET: false
ISSUES: config mismatch
FIX_PLAN: Update config
FILES:
app/config.toml
---
[server]
port = 8080
---`

	result := prompt.ParseIntentResponse(text)

	if len(result.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(result.Files))
	}
	if result.Files[0].Path != "app/config.toml" {
		t.Errorf("Files[0].Path = %q", result.Files[0].Path)
	}
}

func TestParseIntentResponse_MissingTrailingDelimiter(t *testing.T) {
	text := `INTENT_MET: false
ISSUES: wrong replicas
FIX_PLAN: Fix deployment
FILES:
deploy/patch.yml
---
replicas: 3`

	result := prompt.ParseIntentResponse(text)

	if len(result.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(result.Files))
	}
	if result.Files[0].Path != "deploy/patch.yml" {
		t.Errorf("Files[0].Path = %q", result.Files[0].Path)
	}
}
