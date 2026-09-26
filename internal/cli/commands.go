package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

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

// RenderHistory renders an Application's Revisions as the history table.
func RenderHistory(revisions []corev1alpha1.Revision) string {
	rows := make([][]string, 0, len(revisions))
	for _, rev := range revisions {
		approvedBy := ""
		if rev.Status.Approval != nil {
			approvedBy = rev.Status.Approval.ApprovedBy
		}
		rows = append(rows, []string{rev.Name, string(rev.Status.Phase), rev.Spec.Source.Revision, approvedBy})
	}
	return renderTable([]string{"NAME", "PHASE", "REVISION", "APPROVED BY"}, rows)
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
