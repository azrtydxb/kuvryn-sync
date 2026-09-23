package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
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
