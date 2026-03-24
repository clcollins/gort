package metrics_test

import (
	"testing"

	"github.com/clcollins/gort/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// collectMetric verifies that Collect and Describe do not panic.
// CounterVec/HistogramVec yield zero metrics until a label set is created,
// so we do not assert count > 0 here.
func collectMetric(t *testing.T, c prometheus.Collector) {
	t.Helper()
	ch := make(chan prometheus.Metric, 10)
	c.Collect(ch)
	close(ch)

	desc := make(chan *prometheus.Desc, 10)
	c.Describe(desc)
	close(desc)
}

func TestWebhookRequestsTotal_Registered(t *testing.T) {
	collectMetric(t, metrics.WebhookRequestsTotal)
}

func TestReconcilePollsTotal_Registered(t *testing.T) {
	collectMetric(t, metrics.ReconcilePollsTotal)
}

func TestReconcileDurationSeconds_Registered(t *testing.T) {
	collectMetric(t, metrics.ReconcileDurationSeconds)
}

func TestIntentValidationTotal_Registered(t *testing.T) {
	collectMetric(t, metrics.IntentValidationTotal)
}

func TestAIRequestsTotal_Registered(t *testing.T) {
	collectMetric(t, metrics.AIRequestsTotal)
}

func TestAIRequestDurationSeconds_Registered(t *testing.T) {
	collectMetric(t, metrics.AIRequestDurationSeconds)
}

func TestVCSRequestsTotal_Registered(t *testing.T) {
	collectMetric(t, metrics.VCSRequestsTotal)
}

func TestFixPRsOpenedTotal_Registered(t *testing.T) {
	collectMetric(t, metrics.FixPRsOpenedTotal)
}

func TestFixPRsFailedTotal_Registered(t *testing.T) {
	collectMetric(t, metrics.FixPRsFailedTotal)
}

func TestIncrementWebhookRequests(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(metrics.WebhookRequestsTotal)

	metrics.WebhookRequestsTotal.WithLabelValues("success").Inc()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(mfs) == 0 {
		t.Fatal("expected at least one metric family")
	}
}
