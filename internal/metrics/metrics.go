// Package metrics defines all Prometheus metrics for GORT.
// All metrics are package-level variables registered with the default registry.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// WebhookRequestsTotal counts incoming webhook events by outcome status.
	WebhookRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gort_webhook_requests_total",
			Help: "Total number of webhook requests received, partitioned by status (success, invalid, error).",
		},
		[]string{"status"},
	)

	// ReconcilePollsTotal counts Flux reconciliation poll completions by watcher and result.
	ReconcilePollsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gort_reconcile_polls_total",
			Help: "Total number of Flux reconciliation polls completed, partitioned by watcher and result (success, failure, timeout).",
		},
		[]string{"watcher", "result"},
	)

	// ReconcileDurationSeconds measures how long each reconciliation watch takes.
	ReconcileDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gort_reconcile_duration_seconds",
			Help:    "Duration in seconds of Flux reconciliation watches, partitioned by watcher.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"watcher"},
	)

	// IntentValidationTotal counts intent validation checks by watcher and result.
	IntentValidationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gort_intent_validation_total",
			Help: "Total number of intent validation checks, partitioned by watcher and result (met, not_met, error).",
		},
		[]string{"watcher", "result"},
	)

	// AIRequestsTotal counts AI API calls by operation and status.
	AIRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gort_ai_requests_total",
			Help: "Total number of AI API requests, partitioned by operation (analyze, validate_intent) and status (success, error).",
		},
		[]string{"operation", "status"},
	)

	// AIRequestDurationSeconds measures AI API call latency by operation.
	AIRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gort_ai_request_duration_seconds",
			Help:    "Duration in seconds of AI API requests, partitioned by operation.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// VCSRequestsTotal counts VCS API calls by operation and status.
	VCSRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gort_vcs_requests_total",
			Help: "Total number of VCS API requests, partitioned by operation and status (success, error).",
		},
		[]string{"operation", "status"},
	)

	// FixPRsOpenedTotal counts fix PRs opened by watcher and reason.
	FixPRsOpenedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gort_fix_prs_opened_total",
			Help: "Total number of fix PRs opened, partitioned by watcher and reason (flux_failure, intent_not_met).",
		},
		[]string{"watcher", "reason"},
	)

	// FixPRsFailedTotal counts fix PR creation failures by watcher and reason.
	FixPRsFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gort_fix_prs_failed_total",
			Help: "Total number of fix PR creation failures, partitioned by watcher and reason.",
		},
		[]string{"watcher", "reason"},
	)
)

func init() {
	prometheus.MustRegister(
		WebhookRequestsTotal,
		ReconcilePollsTotal,
		ReconcileDurationSeconds,
		IntentValidationTotal,
		AIRequestsTotal,
		AIRequestDurationSeconds,
		VCSRequestsTotal,
		FixPRsOpenedTotal,
		FixPRsFailedTotal,
	)
}
