package flux_test

import (
	"context"
	"testing"
	"time"

	"github.com/clcollins/gort/internal/flux"
	"github.com/clcollins/gort/internal/k8s"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := kustomizev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func readyKustomization(name, namespace string) *kustomizev1.Kustomization {
	return &kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status: kustomizev1.KustomizationStatus{
			Conditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					Reason:             "ReconciliationSucceeded",
					Message:            "Applied revision: main/abc123",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}
}

func failedKustomization(name, namespace string) *kustomizev1.Kustomization {
	return &kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status: kustomizev1.KustomizationStatus{
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "ReconciliationFailed",
					Message: "kustomize build failed: no such file",
				},
			},
		},
	}
}

func fakeK8sClient(t *testing.T, objs ...runtime.Object) k8s.Client {
	t.Helper()
	scheme := buildScheme(t)
	inner := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()
	return k8s.NewClient(inner)
}

func TestGetReconciliationStatus_Ready(t *testing.T) {
	ks := readyKustomization("cluster-config", "flux-system")
	c := flux.NewClient(fakeK8sClient(t, ks))

	status, err := c.GetReconciliationStatus(context.Background(), "cluster-config", "flux-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Ready {
		t.Error("expected Ready=true")
	}
	if status.Reason != "ReconciliationSucceeded" {
		t.Errorf("reason: got %q", status.Reason)
	}
}

func TestGetReconciliationStatus_Failed(t *testing.T) {
	ks := failedKustomization("cluster-config", "flux-system")
	c := flux.NewClient(fakeK8sClient(t, ks))

	status, err := c.GetReconciliationStatus(context.Background(), "cluster-config", "flux-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Ready {
		t.Error("expected Ready=false")
	}
	if status.Reason != "ReconciliationFailed" {
		t.Errorf("reason: got %q", status.Reason)
	}
}

func TestGetReconciliationStatus_NotFound(t *testing.T) {
	c := flux.NewClient(fakeK8sClient(t))
	_, err := c.GetReconciliationStatus(context.Background(), "missing", "flux-system")
	if err == nil {
		t.Fatal("expected error for missing kustomization")
	}
}

func TestGetManagedResources_Empty(t *testing.T) {
	ks := readyKustomization("cluster-config", "flux-system")
	c := flux.NewClient(fakeK8sClient(t, ks))

	resources, err := c.GetManagedResources(context.Background(), "cluster-config", "flux-system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No inventory entries on a fresh object.
	_ = resources
}

func TestGetRuntimeState_WithPods(t *testing.T) {
	ks := readyKustomization("cluster-config", "flux-system")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-pod",
			Namespace: "default",
			Labels:    map[string]string{"kustomize.toolkit.fluxcd.io/name": "cluster-config"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	c := flux.NewClient(fakeK8sClient(t, ks, pod))

	state, err := c.GetRuntimeState(context.Background(), "cluster-config", "flux-system")
	if err != nil {
		t.Fatalf("GetRuntimeState: %v", err)
	}
	if len(state.Pods) == 0 {
		t.Error("expected at least one pod in runtime state")
	}
}

func TestWatchReconciliation_ImmediateSuccess(t *testing.T) {
	ks := readyKustomization("cluster-config", "flux-system")
	c := flux.NewClient(fakeK8sClient(t, ks))

	result, err := c.WatchReconciliation(context.Background(), "cluster-config", "flux-system", 5*time.Second)
	if err != nil {
		t.Fatalf("WatchReconciliation: %v", err)
	}
	if !result.Succeeded {
		t.Errorf("expected succeeded=true, status: %+v", result.Status)
	}
}

func TestWatchReconciliation_ImmediateFailure(t *testing.T) {
	ks := failedKustomization("cluster-config", "flux-system")
	c := flux.NewClient(fakeK8sClient(t, ks))

	result, err := c.WatchReconciliation(context.Background(), "cluster-config", "flux-system", 5*time.Second)
	if err != nil {
		t.Fatalf("WatchReconciliation: %v", err)
	}
	if result.Succeeded {
		t.Error("expected succeeded=false for failed kustomization")
	}
}
