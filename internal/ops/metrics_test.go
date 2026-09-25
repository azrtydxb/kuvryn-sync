package ops

import (
	"strings"
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/brand"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestMetricNamesUseKuvrynSyncPrefix(t *testing.T) {
	// A vector without samples is not gathered, so record one of each.
	app := corev1alpha1.Application{}
	ObserveReconcile(app, corev1alpha1.RevisionPhaseHealthy, nil, time.Millisecond)
	ObserveLifecycleEvent(app, corev1alpha1.RevisionPhaseHealthy, "Test")
	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	ours := 0
	for _, f := range families {
		if strings.HasPrefix(f.GetName(), brand.OldName+"_") {
			t.Errorf("metric %s keeps the old prefix", f.GetName())
		}
		if strings.HasPrefix(f.GetName(), "kuvryn_sync_") {
			ours++
		}
	}
	if ours != 3 {
		t.Errorf("gathered %d kuvryn_sync_ metric families, want 3", ours)
	}
}
