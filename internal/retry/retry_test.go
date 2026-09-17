package retry

import (
	"strings"
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

func TestDecideBlocksAfterMaxAttemptsAndReportsHonestly(t *testing.T) {
	max := int32(2)
	decision := Decide(corev1alpha1.FailurePolicy{MaxAttempts: &max}, State{DesiredRevision: "bad", DeployedRevision: "good", Attempts: 2}, time.Now(), 0)
	if decision.Allowed {
		t.Fatal("retry loop allowed after max attempts")
	}
	if decision.DesiredRevision != "bad" || decision.DeployedRevision != "good" || !strings.Contains(decision.Reason, "maxAttempts") {
		t.Fatalf("dishonest decision: %#v", decision)
	}
}

func TestDecideHonorsBackoffAndSuspension(t *testing.T) {
	now := time.Unix(100, 0)
	decision := Decide(corev1alpha1.FailurePolicy{}, State{DesiredRevision: "bad", LastFailureAt: now, Attempts: 0}, now.Add(time.Second), time.Minute)
	if decision.Allowed || decision.NextAttemptAfter.IsZero() {
		t.Fatalf("backoff not enforced: %#v", decision)
	}
	decision = Decide(corev1alpha1.FailurePolicy{}, State{Suspended: true}, now, 0)
	if decision.Allowed || decision.Reason == "" {
		t.Fatalf("suspension not enforced: %#v", decision)
	}
}

func TestReportHonestRevisions(t *testing.T) {
	app := &corev1alpha1.Application{}
	ReportHonestRevisions(app, "desired", "deployed")
	if app.Status.DesiredRevision != "desired" || app.Status.DeployedRevision != "deployed" {
		t.Fatalf("status = %#v", app.Status)
	}
}
