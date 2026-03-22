// Package gitops defines the interface and types for interacting with a GitOps engine
// (e.g. Flux, ArgoCD) running in a Kubernetes cluster.
package gitops

import "time"

// ReconciliationStatus represents the current state of a GitOps reconciliation.
type ReconciliationStatus struct {
	Name        string
	Namespace   string
	Ready       bool
	Reason      string
	Message     string
	LastApplied time.Time
}

// ReconciliationResult is the terminal outcome of watching a reconciliation.
type ReconciliationResult struct {
	Status    ReconciliationStatus
	Succeeded bool
	Logs      []LogEntry
	Resources []ManagedResource
}

// LogEntry is a single structured log line from a GitOps engine or managed pod.
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Source    string
}

// ManagedResource describes a Kubernetes resource managed by the GitOps engine.
type ManagedResource struct {
	Group     string
	Version   string
	Kind      string
	Name      string
	Namespace string
	Ready     bool
	Message   string
}

// RuntimeState is a snapshot of the running Kubernetes environment for a GitOps app.
type RuntimeState struct {
	Pods        []PodState
	Deployments []DeploymentState
	Events      []EventEntry
	Endpoints   []EndpointState
}

// PodState describes the readiness of a single pod.
type PodState struct {
	Name      string
	Namespace string
	Ready     bool
	Phase     string
	Message   string
}

// DeploymentState describes the replica status of a Deployment.
type DeploymentState struct {
	Name            string
	Namespace       string
	DesiredReplicas int32
	ReadyReplicas   int32
	Message         string
}

// EventEntry is a Kubernetes event relevant to the GitOps app.
type EventEntry struct {
	Reason        string
	Message       string
	Type          string
	Count         int32
	LastTimestamp time.Time
}

// EndpointState describes the readiness of a Service's endpoints.
type EndpointState struct {
	Name      string
	Namespace string
	Ready     bool
	Addresses []string
}
