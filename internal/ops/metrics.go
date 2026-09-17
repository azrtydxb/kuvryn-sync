package ops

import (
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "solder_application_reconcile_total",
		Help: "Total Application reconciliations by bounded lifecycle labels.",
	}, []string{"namespace", "sync", "health", "phase", "result"})

	ReconcileDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "solder_application_reconcile_duration_seconds",
		Help:    "Application reconciliation duration by bounded lifecycle labels.",
		Buckets: prometheus.DefBuckets,
	}, []string{"namespace", "sync", "health", "phase", "result"})

	LifecycleEventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "solder_lifecycle_events_total",
		Help: "Total bounded Solder lifecycle events.",
	}, []string{"namespace", "sync", "health", "phase", "reason"})
)

func init() {
	metrics.Registry.MustRegister(ReconcileTotal, ReconcileDurationSeconds, LifecycleEventsTotal)
}

// ObserveReconcile records one Application reconciliation using bounded labels.
func ObserveReconcile(app corev1alpha1.Application, phase corev1alpha1.RevisionPhase, result string, duration time.Duration) {
	labels := MetricLabels(app, phase)
	result = safeLabel(result, "unknown")
	ReconcileTotal.WithLabelValues(labels["namespace"], labels["sync"], labels["health"], labels["phase"], result).Inc()
	ReconcileDurationSeconds.WithLabelValues(labels["namespace"], labels["sync"], labels["health"], labels["phase"], result).Observe(duration.Seconds())
}

// ObserveLifecycleEvent records a stable lifecycle event with bounded labels.
func ObserveLifecycleEvent(app corev1alpha1.Application, phase corev1alpha1.RevisionPhase, reason string) {
	labels := MetricLabels(app, phase)
	reason = safeLabel(reason, "unknown")
	LifecycleEventsTotal.WithLabelValues(labels["namespace"], labels["sync"], labels["health"], labels["phase"], reason).Inc()
}

type prometheusApplicationMetrics struct {
	started time.Time
}

// PrometheusApplicationMetrics returns the default controller-runtime metrics recorder.
func PrometheusApplicationMetrics() ApplicationMetrics {
	return prometheusApplicationMetrics{started: time.Now()}
}

func (m prometheusApplicationMetrics) ObserveApplication(labels map[string]string, failed bool) {
	result := "success"
	if failed {
		result = "error"
	}
	phase := corev1alpha1.RevisionPhase(labels["phase"])
	app := corev1alpha1.Application{}
	app.Namespace = labels["namespace"]
	app.Status.Sync.State = corev1alpha1.SyncState(labels["sync"])
	app.Status.Health.State = corev1alpha1.HealthState(labels["health"])
	ObserveReconcile(app, phase, result, time.Since(m.started))
}
