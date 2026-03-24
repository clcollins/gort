// Package v1alpha1 contains API Schema definitions for the gitops.gort.io/v1alpha1 group.
// +kubebuilder:object:generate=true
// +groupName=gitops.gort.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GitOpsWatcherSpec defines which GitOps app GORT should watch and where to file fix PRs.
type GitOpsWatcherSpec struct {
	// Type is the GitOps engine: "flux" (future: "argocd").
	// +kubebuilder:validation:Enum=flux
	Type string `json:"type"`

	// AppName is the name of the Flux Kustomization or HelmRelease to watch.
	AppName string `json:"appName"`

	// Namespace is where the Flux resources live (e.g. flux-system).
	Namespace string `json:"namespace"`

	// TargetRepo is the repository GORT watches for push events (e.g. clcollins/cluster-config).
	TargetRepo string `json:"targetRepo"`

	// FixRepo is the repository where GORT opens fix PRs. Usually same as TargetRepo.
	FixRepo string `json:"fixRepo"`

	// DocsPaths are the directory paths inside FixRepo to search for plan documents.
	// Defaults to ["docs/plans/"].
	// +optional
	DocsPaths []string `json:"docsPaths,omitempty"`

	// ReconcileTimeout is how long to wait for Flux reconciliation before declaring timeout.
	// Defaults to 10m.
	// +optional
	ReconcileTimeout *metav1.Duration `json:"reconcileTimeout,omitempty"`
}

// GitOpsWatcherStatus is the observed state of a GitOpsWatcher.
type GitOpsWatcherStatus struct {
	// LastReconcileTime is when GORT last processed a reconciliation for this watcher.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// LastResult is the outcome of the last reconciliation: "success", "fix_pr_opened",
	// or "error".
	// +optional
	LastResult string `json:"lastResult,omitempty"`

	// LastFixPRURL is the URL of the most recently opened fix PR, if any.
	// +optional
	LastFixPRURL string `json:"lastFixPRURL,omitempty"`

	// Conditions holds standard Kubernetes condition types for this watcher.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=gow
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="App",type=string,JSONPath=`.spec.appName`
// +kubebuilder:printcolumn:name="Last Result",type=string,JSONPath=`.status.lastResult`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GitOpsWatcher tells GORT which GitOps application to watch for reconciliation outcomes.
type GitOpsWatcher struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GitOpsWatcherSpec   `json:"spec,omitempty"`
	Status GitOpsWatcherStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GitOpsWatcherList contains a list of GitOpsWatcher.
type GitOpsWatcherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GitOpsWatcher `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GitOpsWatcher{}, &GitOpsWatcherList{})
}
