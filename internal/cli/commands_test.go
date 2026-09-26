package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRenderCoreReadCommands(t *testing.T) {
	apps := RenderApplications([]corev1alpha1.Application{{ObjectMeta: metav1.ObjectMeta{Name: "payments"}, Status: corev1alpha1.ApplicationStatus{Sync: corev1alpha1.ApplicationSyncStatus{State: corev1alpha1.SyncStateSynced}, Health: corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateHealthy}, DesiredRevision: "abc", DeployedRevision: "abc", ServiceAccountName: "payments-deployer"}}})
	if !strings.Contains(apps, "payments") || !strings.Contains(apps, "Synced") || !strings.Contains(apps, "Healthy") || !strings.Contains(apps, "payments-deployer") || !strings.Contains(apps, "SERVICEACCOUNT") {
		t.Fatalf("bad app output: %s", apps)
	}
	repos := RenderRepositories([]corev1alpha1.Repository{{ObjectMeta: metav1.ObjectMeta{Name: "platform"}, Spec: corev1alpha1.RepositorySpec{Type: corev1alpha1.RepositoryTypeGit}, Status: corev1alpha1.RepositoryStatus{State: corev1alpha1.RepositoryStateReady, ObservedRevision: "abc"}}})
	if !strings.Contains(repos, "platform") || !strings.Contains(repos, "Ready") {
		t.Fatalf("bad repo output: %s", repos)
	}
}

func TestRenderHistoryJSONExportsApprovalsAndRedacts(t *testing.T) {
	approvedAt := metav1.NewTime(time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC))
	older := corev1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-old", CreationTimestamp: metav1.NewTime(approvedAt.Add(-time.Hour))},
		Status: corev1alpha1.RevisionStatus{
			Phase:   corev1alpha1.RevisionPhaseFailed,
			Failure: &corev1alpha1.RevisionFailure{Reason: "ApplyFailure", Message: "apply failed: password=hunter2"},
		},
	}
	newer := corev1alpha1.Revision{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-new", CreationTimestamp: approvedAt},
		Spec:       corev1alpha1.RevisionSpec{Source: corev1alpha1.RevisionSource{Revision: "abc123"}},
		Status: corev1alpha1.RevisionStatus{
			Phase:       corev1alpha1.RevisionPhaseHealthy,
			CompletedAt: &approvedAt,
			Plan:        corev1alpha1.RevisionPlan{Digest: "digest-1"},
			Approval:    &corev1alpha1.RevisionApproval{ApprovedBy: "alice@example.com", ApprovedAt: approvedAt, PlanDigest: "digest-1"},
		},
	}

	out, err := RenderHistoryJSON([]corev1alpha1.Revision{newer, older})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "hunter2") {
		t.Fatalf("secret leaked into history export: %s", out)
	}
	var entries []HistoryEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Revision != "payments-old" || entries[1].Revision != "payments-new" {
		t.Fatalf("entries not oldest first: %s", out)
	}
	got := entries[1]
	if got.ApprovedBy != "alice@example.com" || got.PlanDigest != "digest-1" || got.Outcome != "Healthy" || got.CompletedAt == nil || got.SourceRevision != "abc123" {
		t.Fatalf("approved entry = %#v", got)
	}
	if entries[0].FailureReason != "ApplyFailure" {
		t.Fatalf("failed entry = %#v", entries[0])
	}
}

// Catches history listed in the API server's alphabetical order, and an
// approver shown for a Revision whose current plan nobody approved: a
// rollback target back in AwaitingApproval kept its first rollout's
// approval, and history named that approver.
func TestRenderHistoryNewestFirstWithCurrentApprovers(t *testing.T) {
	at := func(minute int) metav1.Time {
		return metav1.NewTime(time.Date(2026, 9, 26, 10, minute, 0, 0, time.UTC))
	}
	revision := func(name string, created int, started *int, phase corev1alpha1.RevisionPhase, digest string, approval *corev1alpha1.RevisionApproval) corev1alpha1.Revision {
		rev := corev1alpha1.Revision{ObjectMeta: metav1.ObjectMeta{Name: name, CreationTimestamp: at(created)}}
		rev.Spec.Source.Revision = strings.TrimPrefix(name, "podinfo-")
		rev.Spec.DesiredStateHash = "hash-" + name
		rev.Status.Phase, rev.Status.Plan.Digest, rev.Status.Approval = phase, digest, approval
		if started != nil {
			s := at(*started)
			rev.Status.StartedAt = &s
		}
		return rev
	}
	approved := func(by, digest, name string) *corev1alpha1.RevisionApproval {
		return &corev1alpha1.RevisionApproval{ApprovedBy: by, ApprovedAt: at(0), PlanDigest: digest, DesiredStateHash: "hash-podinfo-" + name}
	}
	ptr := func(i int) *int { return &i }
	revisions := []corev1alpha1.Revision{
		// Back in AwaitingApproval after a rollback re-planned it: its
		// approval was for the plan of its first rollout.
		revision("podinfo-a", 0, nil, corev1alpha1.RevisionPhaseAwaitingApproval, "digest-replanned", approved("system:admin", "digest-first", "a")),
		// Deployed: re-planning after the rollout changed its plan digest,
		// but the approval covered the rollout it ran.
		revision("podinfo-b", 1, ptr(2), corev1alpha1.RevisionPhaseHealthy, "digest-synced", approved("alice@example.com", "digest-applied", "b")),
		// Deployed on an approval, then its desired state changed, such
		// as a Helm valuesFrom Secret.
		revision("podinfo-c", 3, nil, corev1alpha1.RevisionPhaseHealthy, "digest-new", approved("bob@example.com", "digest-old", "c-before")),
		// Started after it was created, later than podinfo-e.
		revision("podinfo-d", 4, ptr(9), corev1alpha1.RevisionPhaseObserving, "digest-d", approved("carol@example.com", "digest-d", "d")),
		revision("podinfo-e", 5, nil, corev1alpha1.RevisionPhaseFailed, "", nil),
		// A deployed Revision re-planned by the next reconcile passes
		// through Planning; its approval still covers it.
		revision("podinfo-f", 6, nil, corev1alpha1.RevisionPhasePlanning, "digest-replanned", approved("dave@example.com", "digest-applied", "f")),
	}

	lines := strings.Split(strings.TrimSpace(RenderHistory(revisions)), "\n")
	want := []struct{ name, approver string }{
		{"podinfo-d", "carol@example.com"},
		{"podinfo-f", "dave@example.com"},
		{"podinfo-e", "—"},
		{"podinfo-c", "—"},
		{"podinfo-b", "alice@example.com"},
		{"podinfo-a", "—"},
	}
	if len(lines) != len(want)+1 {
		t.Fatalf("history:\n%s", strings.Join(lines, "\n"))
	}
	for i, w := range want {
		fields := strings.Fields(lines[i+1])
		if fields[0] != w.name || fields[len(fields)-1] != w.approver {
			t.Errorf("row %d = %q; want %s approved by %s\n%s", i, lines[i+1], w.name, w.approver, strings.Join(lines, "\n"))
		}
	}

	// The audit export keeps every recorded approval and its fields.
	out, err := RenderHistoryJSON(revisions)
	if err != nil {
		t.Fatal(err)
	}
	var entries []HistoryEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	if entries[0].Revision != "podinfo-a" || entries[0].ApprovedBy != "system:admin" || entries[0].PlanDigest != "digest-first" {
		t.Fatalf("JSON export changed: %#v", entries[0])
	}
}
