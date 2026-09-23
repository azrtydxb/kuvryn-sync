package syncpolicy

import (
	"fmt"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

// Approval requires an operator to name the exact Revision that may be applied.
type Approval struct {
	Revision string
}

// CheckApproval verifies that manual approval targets the planned Revision exactly.
func CheckApproval(plannedRevision string, approval Approval) error {
	if plannedRevision == "" {
		return fmt.Errorf("planned revision is required")
	}
	if approval.Revision == "" {
		return fmt.Errorf("approval must target exact Revision %q", plannedRevision)
	}
	if approval.Revision != plannedRevision {
		return fmt.Errorf("approval targets Revision %q, want %q", approval.Revision, plannedRevision)
	}
	return nil
}

// EnsureMutationAllowed centralizes gates that must pass before apply or prune mutates resources.
func EnsureMutationAllowed(app corev1alpha1.Application, phase corev1alpha1.RevisionPhase) error {
	if app.Spec.Suspend {
		return fmt.Errorf("application %s is suspended", app.Name)
	}
	switch phase {
	case corev1alpha1.RevisionPhaseApplying, corev1alpha1.RevisionPhaseRollingBack:
		return nil
	default:
		return fmt.Errorf("revision phase %q is not allowed to mutate resources", phase)
	}
}

// EffectiveConflictPolicy returns Solder's conservative default conflict behavior.
func EffectiveConflictPolicy(policy corev1alpha1.SyncPolicy) corev1alpha1.ConflictPolicy {
	if policy.ConflictPolicy == "" {
		return corev1alpha1.ConflictPolicyFail
	}
	return policy.ConflictPolicy
}
