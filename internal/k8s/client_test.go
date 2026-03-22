package k8s_test

import (
	"context"
	"strings"
	"testing"

	"github.com/clcollins/gort/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("add corev1: %v", err)
	}
	return s
}

func TestGet_Pod(t *testing.T) {
	scheme := testScheme(t)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	c := k8s.NewClient(fakeClient)

	got := &corev1.Pod{}
	if err := c.Get(context.Background(), "default", "test-pod", got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "test-pod" {
		t.Errorf("got name %q, want %q", got.Name, "test-pod")
	}
}

func TestList_Pods(t *testing.T) {
	scheme := testScheme(t)
	pods := []runtime.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-a", Namespace: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-b", Namespace: "default"}},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(pods...).Build()
	c := k8s.NewClient(fakeClient)

	list := &corev1.PodList{}
	if err := c.List(context.Background(), list, "default"); err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Items) != 2 {
		t.Errorf("got %d pods, want 2", len(list.Items))
	}
}

func TestGetEvents(t *testing.T) {
	scheme := testScheme(t)
	event := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		Reason:         "Failed",
		Message:        "something went wrong",
		InvolvedObject: corev1.ObjectReference{Name: "pod-a"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(event).Build()
	c := k8s.NewClient(fakeClient)

	events, err := c.GetEvents(context.Background(), "default", "")
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("got %d events, want 1", len(events))
	}
	if !strings.Contains(events[0].Message, "something went wrong") {
		t.Errorf("unexpected event message: %q", events[0].Message)
	}
}
