package syncpolicy

import (
	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// EffectiveConflictPolicy returns Solder's conservative default conflict behavior.
func EffectiveConflictPolicy(policy corev1alpha1.SyncPolicy) corev1alpha1.ConflictPolicy {
	if policy.ConflictPolicy == "" {
		return corev1alpha1.ConflictPolicyFail
	}
	return policy.ConflictPolicy
}
