package rollback

import (
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTargetFindsPreviousHealthyRevision(t *testing.T) {
	current := rev("cur", "payments", corev1alpha1.RevisionPhaseFailed, 10)
	history := []corev1alpha1.Revision{
		rev("other", "other", corev1alpha1.RevisionPhaseHealthy, 9),
		rev("bad", "payments", corev1alpha1.RevisionPhaseFailed, 8),
		rev("old", "payments", corev1alpha1.RevisionPhaseHealthy, 7),
		rev("healthy", "payments", corev1alpha1.RevisionPhaseHealthy, 9),
	}
	target, err := Target(current, history)
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "healthy" {
		t.Fatalf("target = %s", target.Name)
	}
}

func TestTargetErrorsWhenPreviousHealthyMissing(t *testing.T) {
	_, err := Target(rev("cur", "payments", corev1alpha1.RevisionPhaseFailed, 10), nil)
	if err == nil {
		t.Fatal("expected missing healthy revision error")
	}
}

func rev(name, app string, phase corev1alpha1.RevisionPhase, unix int64) corev1alpha1.Revision {
	return corev1alpha1.Revision{ObjectMeta: metav1.ObjectMeta{Name: name, CreationTimestamp: metav1.NewTime(time.Unix(unix, 0))}, Spec: corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: app}}, Status: corev1alpha1.RevisionStatus{Phase: phase}}
}
