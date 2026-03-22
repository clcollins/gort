package gitops

import (
	"context"
	"time"
)

// Client is the interface for interacting with a GitOps engine (Flux, ArgoCD, etc.).
// All implementations must be safe for concurrent use.
type Client interface {
	// GetReconciliationStatus returns the current reconciliation status for a named app.
	GetReconciliationStatus(ctx context.Context, name, namespace string) (*ReconciliationStatus, error)

	// GetFailureLogs returns structured log entries for a failed reconciliation.
	GetFailureLogs(ctx context.Context, name, namespace string) ([]LogEntry, error)

	// GetManagedResources lists all Kubernetes resources owned by the GitOps app.
	GetManagedResources(ctx context.Context, name, namespace string) ([]ManagedResource, error)

	// WatchReconciliation polls until the reconciliation completes or timeout elapses.
	// It returns the terminal ReconciliationResult.
	WatchReconciliation(ctx context.Context, name, namespace string, timeout time.Duration) (*ReconciliationResult, error)

	// GetRuntimeState collects the live pod/deployment/event/endpoint state for all
	// resources managed by the named GitOps app.
	GetRuntimeState(ctx context.Context, name, namespace string) (*RuntimeState, error)
}
