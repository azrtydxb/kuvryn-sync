package status

import (
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
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

// TestCompleteHealthyKeepsTheFirstCompletion fails if confirming an already
// healthy Revision again - every "already synced" reconcile does, and a
// manager restart forces one - moves its completedAt, which then no longer
// says when the rollout finished.
func TestCompleteHealthyKeepsTheFirstCompletion(t *testing.T) {
	rev := &corev1alpha1.Revision{}
	app := &corev1alpha1.Application{}
	first := metav1.NewTime(time.Date(2026, 9, 25, 19, 48, 25, 0, time.UTC))
	CompleteHealthy(rev, app, first)
	CompleteHealthy(rev, app, metav1.NewTime(first.Add(10*time.Hour)))
	if !rev.Status.CompletedAt.Equal(&first) {
		t.Fatalf("completedAt = %v after a second confirmation, want the first completion %v", rev.Status.CompletedAt, first)
	}
	// A Revision that failed and is then healed completes anew.
	rev = &corev1alpha1.Revision{}
	Fail(rev, app, first, corev1alpha1.RevisionFailure{Reason: "HealthTimeout"})
	healed := metav1.NewTime(first.Add(time.Hour))
	CompleteHealthy(rev, app, healed)
	if !rev.Status.CompletedAt.Equal(&healed) {
		t.Fatalf("completedAt = %v after a failed Revision became healthy, want %v", rev.Status.CompletedAt, healed)
	}
}
