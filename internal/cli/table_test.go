package cli

import (
	"regexp"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// columnGap separates the header's columns: tabwriter pads with at least two
// spaces, and a header such as "APPROVED BY" has only one inside it.
var columnGap = regexp.MustCompile(`\S {2,}`)

// columnStarts returns where each of the header's columns starts.
func columnStarts(header string) []int {
	gaps := columnGap.FindAllStringIndex(header, -1)
	starts := make([]int, 1, len(gaps)+1)
	for _, m := range gaps {
		starts = append(starts, m[1])
	}
	return starts
}

// assertAligned fails when a table line holds a tab, or when a row's cell does
// not start at its header's column: between two column starts, a row holds
// either only spaces or a cell that starts there and ends in padding.
func assertAligned(t *testing.T, name, table string) {
	t.Helper()
	if strings.Contains(table, "\t") {
		t.Fatalf("%s: table contains a tab character:\n%q", name, table)
	}
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("%s: want a header and rows, got:\n%s", name, table)
	}
	starts := columnStarts(lines[0])
	if len(starts) < 2 {
		t.Fatalf("%s: header has fewer than two columns: %q", name, lines[0])
	}
	for _, row := range lines[1:] {
		if len(row) <= starts[1] || row[starts[1]] == ' ' || row[starts[1]-1] != ' ' {
			t.Fatalf("%s: the second column of row %q does not start at offset %d, as in the header %q", name, row, starts[1], lines[0])
		}
		for i, start := range starts {
			end := len(row)
			if i+1 < len(starts) {
				end = min(starts[i+1], len(row))
			}
			if start >= end {
				continue // trailing empty cells
			}
			cell := row[start:end]
			if strings.TrimSpace(cell) == "" {
				continue
			}
			padded := i+1 == len(starts) || strings.HasSuffix(cell, "  ")
			if cell[0] == ' ' || !padded {
				t.Fatalf("%s: column %d of row %q is %q, not aligned under the header %q", name, i+1, row, cell, lines[0])
			}
		}
	}
}

// Catches tab-separated table output, which is ragged in a terminal, in every
// command that prints a table.
func TestTablesAreAligned(t *testing.T) {
	approved := &corev1alpha1.RevisionApproval{ApprovedBy: "alice@example.com"}
	revisions := []corev1alpha1.Revision{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "podinfo-b939e830aae1"},
			Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: "podinfo"}, Source: corev1alpha1.RevisionSource{Revision: "b939e830aae1f00d"}},
			Status:     corev1alpha1.RevisionStatus{Phase: corev1alpha1.RevisionPhaseAwaitingApproval},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "podinfo-a30f"},
			Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: "podinfo"}, Source: corev1alpha1.RevisionSource{Revision: "a30f"}},
			Status:     corev1alpha1.RevisionStatus{Phase: corev1alpha1.RevisionPhaseHealthy, Approval: approved},
		},
	}
	tables := map[string]string{
		"apps": RenderApplications([]corev1alpha1.Application{
			{ObjectMeta: metav1.ObjectMeta{Name: "podinfo"}, Status: corev1alpha1.ApplicationStatus{Sync: corev1alpha1.ApplicationSyncStatus{State: corev1alpha1.SyncStateAwaitingApproval}, Health: corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateUnknown}, DesiredRevision: "a30f1c2e", ServiceAccountName: "podinfo-deployer"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "a-much-longer-application-name"}, Status: corev1alpha1.ApplicationStatus{Sync: corev1alpha1.ApplicationSyncStatus{State: corev1alpha1.SyncStateSynced}, Health: corev1alpha1.ApplicationHealthStatus{State: corev1alpha1.HealthStateHealthy}, DesiredRevision: "b939e830", DeployedRevision: "b939e830", ServiceAccountName: "sa"}},
		}),
		"repos": RenderRepositories([]corev1alpha1.Repository{
			{ObjectMeta: metav1.ObjectMeta{Name: "platform"}, Spec: corev1alpha1.RepositorySpec{Type: corev1alpha1.RepositoryTypeGit}, Status: corev1alpha1.RepositoryStatus{State: corev1alpha1.RepositoryStateReady, ObservedRevision: "abc"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "podinfo-source"}, Spec: corev1alpha1.RepositorySpec{Type: corev1alpha1.RepositoryTypeGit}, Status: corev1alpha1.RepositoryStatus{State: corev1alpha1.RepositoryStateFailed}},
		}),
		"history":  RenderHistory(revisions),
		"revision": RenderRevision(revisions[0]),
	}
	for name, table := range tables {
		assertAligned(t, name, table)
	}
	for name, header := range map[string][]string{
		"apps":     {"NAME", "SYNC", "HEALTH", "DESIRED", "DEPLOYED", "SERVICEACCOUNT"},
		"repos":    {"NAME", "TYPE", "STATE", "REVISION"},
		"history":  {"NAME", "PHASE", "REVISION", "APPROVED BY"},
		"revision": {"NAME", "PHASE", "APPLICATION", "REVISION"},
	} {
		first := strings.SplitN(tables[name], "\n", 2)[0]
		if got := regexp.MustCompile(` {2,}`).Split(first, -1); strings.Join(got, "|") != strings.Join(header, "|") {
			t.Errorf("%s header columns = %q, want %q", name, got, header)
		}
	}
}
