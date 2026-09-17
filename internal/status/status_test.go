package status

import (
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLifecycleTransitionsKeepSyncAndHealthSeparate(t *testing.T) {
	now := metav1.NewTime(time.Unix(10, 0))
	rev := &corev1alpha1.Revision{Spec: corev1alpha1.RevisionSpec{Source: corev1alpha1.RevisionSource{Revision: "abc123"}}}
	app := &corev1alpha1.Application{}
	StartPlanning(rev, app, now)
	if rev.Status.Phase != corev1alpha1.RevisionPhasePlanning || app.Status.Sync.State != corev1alpha1.SyncStatePlanning {
		t.Fatalf("planning transition failed rev=%s app=%s", rev.Status.Phase, app.Status.Sync.State)
	}
	if app.Status.Health.State != corev1alpha1.HealthStateUnknown {
		t.Fatalf("planning changed health unexpectedly: %s", app.Status.Health.State)
	}
	AwaitApproval(rev, app)
	StartApplying(rev, app, now)
	if app.Status.Health.State != corev1alpha1.HealthStateUnknown {
		t.Fatalf("applying changed health unexpectedly: %s", app.Status.Health.State)
	}
	CompleteHealthy(rev, app, now)
	if rev.Status.Phase != corev1alpha1.RevisionPhaseHealthy || app.Status.Sync.State != corev1alpha1.SyncStateSynced || app.Status.Health.State != corev1alpha1.HealthStateHealthy {
		t.Fatalf("healthy transition failed rev=%s sync=%s health=%s", rev.Status.Phase, app.Status.Sync.State, app.Status.Health.State)
	}
	if app.Status.DeployedRevision != "abc123" {
		t.Fatalf("deployed revision = %q", app.Status.DeployedRevision)
	}
}

func TestFailRecordsFailureAndDegradedHealth(t *testing.T) {
	now := metav1.Now()
	rev := &corev1alpha1.Revision{}
	app := &corev1alpha1.Application{}
	Fail(rev, app, now, corev1alpha1.RevisionFailure{Reason: "ApplyFailure", Retryable: true})
	if rev.Status.Phase != corev1alpha1.RevisionPhaseFailed || rev.Status.Failure == nil {
		t.Fatalf("failure not recorded: %#v", rev.Status)
	}
	if app.Status.Sync.State != corev1alpha1.SyncStateOutOfSync || app.Status.Health.State != corev1alpha1.HealthStateDegraded {
		t.Fatalf("app states = sync %s health %s", app.Status.Sync.State, app.Status.Health.State)
	}
}
