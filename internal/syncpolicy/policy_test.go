package syncpolicy

import (
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckApprovalRequiresExactRevision(t *testing.T) {
	if err := CheckApproval("rev-a", Approval{Revision: "rev-a"}); err != nil {
		t.Fatalf("exact approval rejected: %v", err)
	}
	if err := CheckApproval("rev-a", Approval{}); err == nil {
		t.Fatal("missing approval accepted")
	}
	if err := CheckApproval("rev-a", Approval{Revision: "rev-b"}); err == nil {
		t.Fatal("wrong revision accepted")
	}
}

func TestEnsureMutationAllowedHonorsSuspendAndPhase(t *testing.T) {
	app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments"}}
	if err := EnsureMutationAllowed(app, corev1alpha1.RevisionPhaseApplying); err != nil {
		t.Fatalf("applying rejected: %v", err)
	}
	if err := EnsureMutationAllowed(app, corev1alpha1.RevisionPhaseAwaitingApproval); err == nil {
		t.Fatal("awaiting approval allowed mutation")
	}
	app.Spec.Suspend = true
	if err := EnsureMutationAllowed(app, corev1alpha1.RevisionPhaseApplying); err == nil {
		t.Fatal("suspended application allowed mutation")
	}
}

func TestEffectiveConflictPolicyDefaultsToFail(t *testing.T) {
	if got := EffectiveConflictPolicy(corev1alpha1.SyncPolicy{}); got != corev1alpha1.ConflictPolicyFail {
		t.Fatalf("policy = %q", got)
	}
}
