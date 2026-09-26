package cli

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/redact"
)

// renderTable renders a header and rows as columns aligned with spaces, so
// they line up in a terminal.
func renderTable(header []string, rows [][]string) string {
	var b bytes.Buffer
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(header, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	_ = w.Flush()
	return b.String()
}

// RenderApplications renders core Application read output from CRD objects.
func RenderApplications(apps []corev1alpha1.Application) string {
	rows := make([][]string, 0, len(apps))
	for _, app := range apps {
		rows = append(rows, []string{app.Name, string(app.Status.Sync.State), string(app.Status.Health.State), app.Status.DesiredRevision, app.Status.DeployedRevision, app.Status.ServiceAccountName})
	}
	return renderTable([]string{"NAME", "SYNC", "HEALTH", "DESIRED", "DEPLOYED", "SERVICEACCOUNT"}, rows)
}

// RenderRepositories renders core Repository read output from CRD objects.
func RenderRepositories(repos []corev1alpha1.Repository) string {
	rows := make([][]string, 0, len(repos))
	for _, repo := range repos {
		rows = append(rows, []string{repo.Name, string(repo.Spec.Type), string(repo.Status.State), repo.Status.ObservedRevision})
	}
	return renderTable([]string{"NAME", "TYPE", "STATE", "REVISION"}, rows)
}

// noApprover fills the APPROVED BY column of a Revision whose current plan
// no approval covers.
const noApprover = "—"

// RenderHistory renders an Application's Revisions as the history table,
// newest first.
func RenderHistory(revisions []corev1alpha1.Revision) string {
	sorted := append([]corev1alpha1.Revision(nil), revisions...)
	newestFirst(sorted)
	rows := make([][]string, 0, len(sorted))
	for i := range sorted {
		rev := &sorted[i]
		approvedBy := noApprover
		if approvalCovers(rev) {
			approvedBy = rev.Status.Approval.ApprovedBy
		}
		rows = append(rows, []string{rev.Name, string(rev.Status.Phase), rev.Spec.Source.Revision, approvedBy})
	}
	return renderTable([]string{"NAME", "PHASE", "REVISION", "APPROVED BY"}, rows)
}

// startOf is when a Revision's rollout started, or its creation before one
// has.
func startOf(rev *corev1alpha1.Revision) time.Time {
	if rev.Status.StartedAt != nil {
		return rev.Status.StartedAt.Time
	}
	return rev.CreationTimestamp.Time
}

// newestFirst sorts Revisions newest first by start, then by name, as the
// console does: creation timestamps have one-second resolution.
func newestFirst(revisions []corev1alpha1.Revision) {
	slices.SortStableFunc(revisions, func(a, b corev1alpha1.Revision) int {
		if c := startOf(&b).Compare(startOf(&a)); c != 0 {
			return c
		}
		return cmp.Compare(b.Name, a.Name)
	})
}

// approvalCovers reports whether the approval recorded on a Revision covers
// its current plan. The controller records an approval only when a rollout
// acts on it, and re-plans against the live state that rollout produced, so
// the plan digest of a deployed Revision moves on from the approved one: a
// digest mismatch alone does not make an approval stale. One awaiting
// approval is not covered: any approval it still carries was for an earlier
// plan, such as its first rollout's before a rollback re-planned it. Nor is
// one whose desired state changed since it was approved.
func approvalCovers(rev *corev1alpha1.Revision) bool {
	approval := rev.Status.Approval
	if approval == nil || approval.ApprovedBy == "" || rev.Status.Phase == corev1alpha1.RevisionPhaseAwaitingApproval {
		return false
	}
	return approval.DesiredStateHash == "" || approval.DesiredStateHash == rev.Spec.DesiredStateHash
}

// RenderRevision renders one Revision.
func RenderRevision(rev corev1alpha1.Revision) string {
	return renderTable([]string{"NAME", "PHASE", "APPLICATION", "REVISION"}, [][]string{{rev.Name, string(rev.Status.Phase), rev.Spec.ApplicationRef.Name, rev.Spec.Source.Revision}})
}

// HistoryEntry is the audit export of one Revision.
type HistoryEntry struct {
	Revision       string       `json:"revision"`
	SourceRevision string       `json:"sourceRevision"`
	Outcome        string       `json:"outcome"`
	FailureReason  string       `json:"failureReason,omitempty"`
	FailureMessage string       `json:"failureMessage,omitempty"`
	PlanDigest     string       `json:"planDigest,omitempty"`
	ApprovedBy     string       `json:"approvedBy,omitempty"`
	ApprovedAt     *metav1.Time `json:"approvedAt,omitempty"`
	StartedAt      *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt    *metav1.Time `json:"completedAt,omitempty"`
}

// RenderHistoryJSON exports Revisions, oldest first, as redacted JSON for audit.
func RenderHistoryJSON(revisions []corev1alpha1.Revision) (string, error) {
	sorted := append([]corev1alpha1.Revision(nil), revisions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreationTimestamp.Before(&sorted[j].CreationTimestamp)
	})
	entries := make([]HistoryEntry, 0, len(sorted))
	for _, rev := range sorted {
		entry := HistoryEntry{
			Revision:       rev.Name,
			SourceRevision: rev.Spec.Source.Revision,
			Outcome:        string(rev.Status.Phase),
			PlanDigest:     rev.Status.Plan.Digest,
			StartedAt:      rev.Status.StartedAt,
			CompletedAt:    rev.Status.CompletedAt,
		}
		if failure := rev.Status.Failure; failure != nil {
			entry.FailureReason = failure.Reason
			entry.FailureMessage = redact.String(failure.Message)
		}
		if approval := rev.Status.Approval; approval != nil {
			entry.ApprovedBy = approval.ApprovedBy
			entry.ApprovedAt = &approval.ApprovedAt
			entry.PlanDigest = approval.PlanDigest
		}
		entries = append(entries, entry)
	}
	out, err := json.MarshalIndent(entries, "", "  ")
	return string(out), err
}
