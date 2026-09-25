package ops

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMetricLabelsAreBounded(t *testing.T) {
	app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "payments"}, Status: corev1alpha1.ApplicationStatus{Sync: corev1alpha1.ApplicationSyncStatus{State: corev1alpha1.SyncStateSynced}, Health: corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateHealthy}}}
	labels := MetricLabels(app, corev1alpha1.RevisionPhaseHealthy)
	wantKeys := []string{"health", "namespace", "phase", "sync"}
	if keys := slices.Sorted(maps.Keys(labels)); !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("keys = %#v", keys)
	}
	if labels["namespace"] != "payments" || labels["sync"] != "Synced" {
		t.Fatalf("labels = %#v", labels)
	}
}
