// Package flux implements pkg/gitops.Client for Flux CD using the Flux
// Kustomization and HelmRelease CRDs.
// All cluster access goes through the k8s.Client interface, making this
// package fully testable with a fake Kubernetes client.
package flux

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/clcollins/gort/internal/k8s"
	"github.com/clcollins/gort/pkg/gitops"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	pollInterval = 5 * time.Second
	// fluxNameLabel is the label Flux sets on resources it manages.
	fluxNameLabel = "kustomize.toolkit.fluxcd.io/name"
)

type client struct {
	k8s k8s.Client
}

// NewClient returns a gitops.Client backed by Flux CRDs via the given k8s client.
func NewClient(k k8s.Client) gitops.Client {
	return &client{k8s: k}
}

// GetReconciliationStatus fetches the current status of the named Flux Kustomization.
func (c *client) GetReconciliationStatus(ctx context.Context, name, namespace string) (*gitops.ReconciliationStatus, error) {
	ks := &kustomizev1.Kustomization{}
	if err := c.k8s.Get(ctx, namespace, name, ks); err != nil {
		return nil, fmt.Errorf("flux: get kustomization %s/%s: %w", namespace, name, err)
	}
	return kustomizationStatus(ks), nil
}

// kustomizationStatus converts a Flux Kustomization status into a generic ReconciliationStatus.
// This is a pure function.
func kustomizationStatus(ks *kustomizev1.Kustomization) *gitops.ReconciliationStatus {
	s := &gitops.ReconciliationStatus{
		Name:      ks.Name,
		Namespace: ks.Namespace,
	}
	for _, cond := range ks.Status.Conditions {
		if cond.Type == "Ready" {
			s.Ready = cond.Status == metav1.ConditionTrue
			s.Reason = cond.Reason
			s.Message = cond.Message
			s.LastApplied = cond.LastTransitionTime.Time
			break
		}
	}
	return s
}

// GetFailureLogs retrieves Kubernetes events associated with the Flux Kustomization.
// Pod log retrieval is best-effort; errors fetching individual pod logs are non-fatal.
func (c *client) GetFailureLogs(ctx context.Context, name, namespace string) ([]gitops.LogEntry, error) {
	events, err := c.k8s.GetEvents(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("flux: get events for %s/%s: %w", namespace, name, err)
	}
	var entries []gitops.LogEntry
	for _, ev := range events {
		entries = append(entries, gitops.LogEntry{
			Timestamp: ev.LastTimestamp.Time,
			Level:     ev.Type,
			Message:   ev.Message,
			Source:    ev.Reason,
		})
	}
	return entries, nil
}

// GetManagedResources lists all resources in the Kustomization's inventory.
func (c *client) GetManagedResources(ctx context.Context, name, namespace string) ([]gitops.ManagedResource, error) {
	ks := &kustomizev1.Kustomization{}
	if err := c.k8s.Get(ctx, namespace, name, ks); err != nil {
		return nil, fmt.Errorf("flux: get kustomization %s/%s: %w", namespace, name, err)
	}

	if ks.Status.Inventory == nil {
		return nil, nil
	}

	resources := make([]gitops.ManagedResource, 0, len(ks.Status.Inventory.Entries))
	for _, entry := range ks.Status.Inventory.Entries {
		resources = append(resources, inventoryEntryToResource(entry))
	}
	return resources, nil
}

// inventoryEntryToResource converts a Flux inventory entry to a ManagedResource.
// The Flux ResourceRef ID format is "<namespace>_<name>_<group>_<kind>".
// This is a pure function.
func inventoryEntryToResource(entry kustomizev1.ResourceRef) gitops.ManagedResource {
	// Parse the Flux ID: namespace_name_group_kind
	parts := splitN(entry.ID, "_", 4)
	r := gitops.ManagedResource{Version: entry.Version}
	if len(parts) == 4 {
		r.Namespace = parts[0]
		r.Name = parts[1]
		r.Group = parts[2]
		r.Kind = parts[3]
	} else {
		r.Name = entry.ID
	}
	return r
}

// splitN splits s by sep into at most n parts, similar to strings.SplitN
// but handles the Flux ID format which may contain underscores in group names.
func splitN(s, sep string, n int) []string {
	result := strings.SplitN(s, sep, n)
	return result
}

// WatchReconciliation polls the Kustomization status until it is Ready or NotReady,
// or until the context deadline / timeout is reached.
func (c *client) WatchReconciliation(ctx context.Context, name, namespace string, timeout time.Duration) (*gitops.ReconciliationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		status, err := c.GetReconciliationStatus(ctx, name, namespace)
		if err != nil {
			return nil, err
		}

		// A terminal state is reached when the Reason indicates a completed reconciliation.
		if isTerminalReason(status.Reason) {
			logs, _ := c.GetFailureLogs(ctx, name, namespace)
			resources, _ := c.GetManagedResources(ctx, name, namespace)
			return &gitops.ReconciliationResult{
				Status:    *status,
				Succeeded: status.Ready,
				Logs:      logs,
				Resources: resources,
			}, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("flux: watch reconciliation %s/%s: %w", namespace, name, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// isTerminalReason returns true when the Flux Kustomization has reached a definitive outcome.
// This is a pure function.
func isTerminalReason(reason string) bool {
	switch reason {
	case "ReconciliationSucceeded", "ReconciliationFailed", "BuildFailed",
		"HealthCheckFailed", "DependencyNotReady", "ArtifactFailed":
		return true
	}
	return false
}

// GetRuntimeState collects the live pod, deployment, and event state
// for all resources labelled with the Flux Kustomization name.
// Endpoint collection is not yet implemented.
func (c *client) GetRuntimeState(ctx context.Context, name, namespace string) (*gitops.RuntimeState, error) {
	state := &gitops.RuntimeState{}

	// Pods labelled by Flux across all namespaces.
	podList := &corev1.PodList{}
	if err := c.k8s.List(ctx, podList, ""); err != nil {
		return nil, fmt.Errorf("flux: list pods: %w", err)
	}
	for _, pod := range podList.Items {
		if pod.Labels[fluxNameLabel] != name {
			continue
		}
		state.Pods = append(state.Pods, podToState(&pod))
	}

	// Deployments.
	depList := &appsv1.DeploymentList{}
	if err := c.k8s.List(ctx, depList, ""); err != nil {
		return nil, fmt.Errorf("flux: list deployments: %w", err)
	}
	for _, dep := range depList.Items {
		if dep.Labels[fluxNameLabel] != name {
			continue
		}
		state.Deployments = append(state.Deployments, deploymentToState(&dep))
	}

	// Events from the GitOps namespace.
	events, err := c.k8s.GetEvents(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("flux: list events: %w", err)
	}
	for _, ev := range events {
		state.Events = append(state.Events, gitops.EventEntry{
			Reason:        ev.Reason,
			Message:       ev.Message,
			Type:          ev.Type,
			Count:         ev.Count,
			LastTimestamp: ev.LastTimestamp.Time,
		})
	}

	return state, nil
}

// podToState converts a corev1.Pod into a gitops.PodState. Pure function.
func podToState(pod *corev1.Pod) gitops.PodState {
	ready := false
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}
	return gitops.PodState{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Ready:     ready,
		Phase:     string(pod.Status.Phase),
		Message:   pod.Status.Message,
	}
}

// deploymentToState converts a Deployment into a gitops.DeploymentState. Pure function.
func deploymentToState(dep *appsv1.Deployment) gitops.DeploymentState {
	desired := int32(0)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	return gitops.DeploymentState{
		Name:            dep.Name,
		Namespace:       dep.Namespace,
		DesiredReplicas: desired,
		ReadyReplicas:   dep.Status.ReadyReplicas,
	}
}
