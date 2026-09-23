package retry

import (
	"strings"
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

func TestDecideBlocksAfterMaxAttempts(t *testing.T) {
	max := int32(2)
	decision := Decide(corev1alpha1.FailurePolicy{MaxAttempts: &max}, State{DesiredRevision: "bad", Attempts: 2}, time.Now(), 0)
	if decision.Allowed {
		t.Fatal("retry loop allowed after max attempts")
	}
	if !strings.Contains(decision.Reason, "maxAttempts") || !strings.Contains(decision.Reason, "bad") {
		t.Fatalf("unexpected reason: %#v", decision)
	}
}

func TestDecideHonorsBackoff(t *testing.T) {
	now := time.Unix(100, 0)
	decision := Decide(corev1alpha1.FailurePolicy{}, State{DesiredRevision: "bad", LastFailureAt: now, Attempts: 0}, now.Add(time.Second), time.Minute)
	if decision.Allowed || decision.Reason == "" {
		t.Fatalf("backoff not enforced: %#v", decision)
	}
	decision = Decide(corev1alpha1.FailurePolicy{}, State{DesiredRevision: "bad", LastFailureAt: now, Attempts: 0}, now.Add(2*time.Minute), time.Minute)
	if !decision.Allowed {
		t.Fatalf("retry blocked after backoff elapsed: %#v", decision)
	}
}
