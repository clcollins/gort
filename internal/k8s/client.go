// Package k8s provides a thin, testable wrapper around the controller-runtime client.
// All cluster access is read-only except for status updates on GitOpsWatcher resources.
package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Client wraps the controller-runtime client with a simplified interface for GORT's needs.
type Client interface {
	// Get retrieves a single named object into obj.
	Get(ctx context.Context, namespace, name string, obj ctrlclient.Object) error

	// List retrieves all objects of the list type in the given namespace.
	List(ctx context.Context, list ctrlclient.ObjectList, namespace string) error

	// GetEvents returns Kubernetes events in the given namespace.
	GetEvents(ctx context.Context, namespace string) ([]corev1.Event, error)

	// UpdateStatus patches the status subresource of the given object.
	UpdateStatus(ctx context.Context, obj ctrlclient.Object) error
}

type client struct {
	inner ctrlclient.Client
}

// NewClient constructs a Client backed by the given controller-runtime client.
func NewClient(inner ctrlclient.Client) Client {
	return &client{inner: inner}
}

func (c *client) Get(ctx context.Context, namespace, name string, obj ctrlclient.Object) error {
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err := c.inner.Get(ctx, key, obj); err != nil {
		return fmt.Errorf("k8s get %s/%s: %w", namespace, name, err)
	}
	return nil
}

func (c *client) List(ctx context.Context, list ctrlclient.ObjectList, namespace string) error {
	opts := []ctrlclient.ListOption{}
	if namespace != "" {
		opts = append(opts, ctrlclient.InNamespace(namespace))
	}
	if err := c.inner.List(ctx, list, opts...); err != nil {
		return fmt.Errorf("k8s list in %q: %w", namespace, err)
	}
	return nil
}

func (c *client) GetEvents(ctx context.Context, namespace string) ([]corev1.Event, error) {
	list := &corev1.EventList{}
	opts := []ctrlclient.ListOption{ctrlclient.InNamespace(namespace)}
	if err := c.inner.List(ctx, list, opts...); err != nil {
		return nil, fmt.Errorf("k8s list events in %q: %w", namespace, err)
	}
	return list.Items, nil
}

func (c *client) UpdateStatus(ctx context.Context, obj ctrlclient.Object) error {
	if err := c.inner.Status().Update(ctx, obj); err != nil {
		return fmt.Errorf("k8s update status: %w", err)
	}
	return nil
}
