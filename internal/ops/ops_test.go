package ops

import (
	"context"
	"reflect"
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLifecycleEventAndMetricLabelsAreBounded(t *testing.T) {
	event := NewLifecycleEvent("payments", "abc", corev1alpha1.RevisionPhaseApplying, "Apply", "message")
	if event.Application != "payments" || event.Phase != corev1alpha1.RevisionPhaseApplying {
		t.Fatalf("event = %#v", event)
	}
	app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "payments"}, Status: corev1alpha1.ApplicationStatus{Sync: corev1alpha1.ApplicationSyncStatus{State: corev1alpha1.SyncStateSynced}, Health: corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateHealthy}}}
	labels := MetricLabels(app, corev1alpha1.RevisionPhaseHealthy)
	wantKeys := []string{"health", "namespace", "phase", "sync"}
	if !reflect.DeepEqual(StableLabelKeys(labels), wantKeys) {
		t.Fatalf("keys = %#v", StableLabelKeys(labels))
	}
	if labels["namespace"] != "payments" || labels["sync"] != "Synced" {
		t.Fatalf("labels = %#v", labels)
	}
}

func TestNoopTracerAndRateLimiter(t *testing.T) {
	ctx, finish := NoopTracer().Start(context.Background(), "operation")
	finish(nil)
	if ctx == nil {
		t.Fatal("nil context")
	}
	limiter := RateLimiter{Base: time.Second, Max: 5 * time.Second}
	if got := limiter.Delay(0); got != time.Second {
		t.Fatalf("delay0 = %s", got)
	}
	if got := limiter.Delay(4); got != 5*time.Second {
		t.Fatalf("delay capped = %s", got)
	}
}

func TestHAAndScaleFixture(t *testing.T) {
	if HALeaderElectionDefault() == "" {
		t.Fatal("missing leader election id")
	}
	if err := (ScaleFixture{Applications: 10, Resources: 100, Revisions: 20}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ScaleFixture{Applications: 10, Resources: 1, Revisions: 20}).Validate(); err == nil {
		t.Fatal("invalid scale fixture accepted")
	}
}
