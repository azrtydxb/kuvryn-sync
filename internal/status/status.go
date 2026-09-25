package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// StartPlanning marks a Revision as planning and keeps Application sync separate from health.
func StartPlanning(rev *corev1alpha1.Revision, app *corev1alpha1.Application, now metav1.Time) {
	rev.Status.Phase = corev1alpha1.RevisionPhasePlanning
	if rev.Status.StartedAt == nil {
		rev.Status.StartedAt = &now
	}
	app.Status.Sync.State = corev1alpha1.SyncStatePlanning
	if app.Status.Health.State == "" {
		app.Status.Health.State = corev1alpha1.HealthStateUnknown
	}
}

// AwaitApproval marks a planned Revision as requiring exact manual approval.
func AwaitApproval(rev *corev1alpha1.Revision, app *corev1alpha1.Application) {
	rev.Status.Phase = corev1alpha1.RevisionPhaseAwaitingApproval
	rev.Status.StartedAt = nil
	app.Status.Sync.State = corev1alpha1.SyncStateAwaitingApproval
}

// StartApplying marks a Revision as mutating resources without changing health state.
func StartApplying(rev *corev1alpha1.Revision, app *corev1alpha1.Application, now metav1.Time) {
	rev.Status.Phase = corev1alpha1.RevisionPhaseApplying
	if rev.Status.StartedAt == nil {
		rev.Status.StartedAt = &now
	}
	app.Status.Sync.State = corev1alpha1.SyncStateApplying
}

// CompleteHealthy marks a Revision deployed and healthy.
func CompleteHealthy(rev *corev1alpha1.Revision, app *corev1alpha1.Application, now metav1.Time) {
	rev.Status.Phase = corev1alpha1.RevisionPhaseHealthy
	rev.Status.CompletedAt = &now
	app.Status.Sync.State = corev1alpha1.SyncStateSynced
	app.Status.Health.State = corev1alpha1.HealthStateHealthy
	app.Status.State = corev1alpha1.HealthStateHealthy
	app.Status.DeployedRevision = rev.Spec.Source.Revision
}

// Fail records a deterministic failure while preserving separate sync and health fields.
func Fail(rev *corev1alpha1.Revision, app *corev1alpha1.Application, now metav1.Time, failure corev1alpha1.RevisionFailure) {
	rev.Status.Phase = corev1alpha1.RevisionPhaseFailed
	rev.Status.CompletedAt = &now
	rev.Status.Failure = &failure
	app.Status.Sync.State = corev1alpha1.SyncStateOutOfSync
	app.Status.Health.State = corev1alpha1.HealthStateDegraded
	app.Status.State = corev1alpha1.HealthStateDegraded
}
