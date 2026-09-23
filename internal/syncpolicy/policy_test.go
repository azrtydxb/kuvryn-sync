package syncpolicy

import (
	"testing"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

func TestEffectiveConflictPolicyDefaultsToFail(t *testing.T) {
	if got := EffectiveConflictPolicy(corev1alpha1.SyncPolicy{}); got != corev1alpha1.ConflictPolicyFail {
		t.Fatalf("policy = %q", got)
	}
}
