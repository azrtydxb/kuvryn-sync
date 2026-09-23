package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/redact"
	"github.com/azrtydxb/solder/internal/syncpolicy"
	"sigs.k8s.io/yaml"
)

// RenderApplications renders core Application read output from CRD objects.
func RenderApplications(apps []corev1alpha1.Application) string {
	var b bytes.Buffer
	_, _ = fmt.Fprintln(&b, "NAME\tSYNC\tHEALTH\tDESIRED\tDEPLOYED\tSERVICEACCOUNT")
	for _, app := range apps {
		_, _ = fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\n", app.Name, app.Status.Sync.State, app.Status.Health.State, app.Status.DesiredRevision, app.Status.DeployedRevision, app.Status.ServiceAccountName)
	}
	return b.String()
}

// RenderRepositories renders core Repository read output from CRD objects.
func RenderRepositories(repos []corev1alpha1.Repository) string {
	var b bytes.Buffer
	_, _ = fmt.Fprintln(&b, "NAME\tTYPE\tSTATE\tREVISION")
	for _, repo := range repos {
		_, _ = fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", repo.Name, repo.Spec.Type, repo.Status.State, repo.Status.ObservedRevision)
	}
	return b.String()
}

// MutationPatch is a public-API patch emitted by a safe mutation command.
type MutationPatch struct {
	Resource string `json:"resource"`
	Name     string `json:"name"`
	Patch    string `json:"patch"`
}

// BuildSyncPatch validates exact approval and returns a public CRD merge patch.
func BuildSyncPatch(app corev1alpha1.Application, plannedRevision, approvedRevision string) (MutationPatch, error) {
	if err := syncpolicy.CheckApproval(plannedRevision, syncpolicy.Approval{Revision: approvedRevision}); err != nil {
		return MutationPatch{}, err
	}
	patch := map[string]any{"metadata": map[string]any{"annotations": map[string]string{"solder.io/approved-revision": approvedRevision}}}
	b, err := yaml.Marshal(patch)
	if err != nil {
		return MutationPatch{}, err
	}
	return MutationPatch{Resource: "applications.solder.io", Name: app.Name, Patch: string(b)}, nil
}

// BuildSuspendPatch returns a public CRD merge patch to suspend or resume an Application.
func BuildSuspendPatch(app corev1alpha1.Application, suspend bool) (MutationPatch, error) {
	patch := map[string]any{"spec": map[string]bool{"suspend": suspend}}
	b, err := yaml.Marshal(patch)
	if err != nil {
		return MutationPatch{}, err
	}
	return MutationPatch{Resource: "applications.solder.io", Name: app.Name, Patch: string(b)}, nil
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
