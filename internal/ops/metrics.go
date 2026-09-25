package ops

import (
	"time"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kuvryn_sync_application_reconcile_total",
		Help: "Total Application reconciliations by bounded lifecycle labels.",
	}, []string{"namespace", "sync", "health", "phase", "result"})

	ReconcileDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "kuvryn_sync_application_reconcile_duration_seconds",
		Help:    "Application reconciliation duration by bounded lifecycle labels.",
		Buckets: prometheus.DefBuckets,
	}, []string{"namespace", "sync", "health", "phase", "result"})

	LifecycleEventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kuvryn_sync_lifecycle_events_total",
		Help: "Total bounded Kuvryn Sync lifecycle events.",
	}, []string{"namespace", "sync", "health", "phase", "reason"})
)

func init() {
	metrics.Registry.MustRegister(ReconcileTotal, ReconcileDurationSeconds, LifecycleEventsTotal)
}

// ObserveReconcile records one Application reconciliation, which took
// duration and failed when err is set, using bounded labels.
func ObserveReconcile(app corev1alpha1.Application, phase corev1alpha1.RevisionPhase, err error, duration time.Duration) {
	labels := MetricLabels(app, phase)
	result := "success"
	if err != nil {
		result = "error"
	}
	ReconcileTotal.WithLabelValues(labels["namespace"], labels["sync"], labels["health"], labels["phase"], result).Inc()
	ReconcileDurationSeconds.WithLabelValues(labels["namespace"], labels["sync"], labels["health"], labels["phase"], result).Observe(duration.Seconds())
}

// ObserveLifecycleEvent records a stable lifecycle event with bounded labels.
func ObserveLifecycleEvent(app corev1alpha1.Application, phase corev1alpha1.RevisionPhase, reason string) {
	labels := MetricLabels(app, phase)
	reason = safeLabel(reason, "unknown")
	LifecycleEventsTotal.WithLabelValues(labels["namespace"], labels["sync"], labels["health"], labels["phase"], reason).Inc()
}
