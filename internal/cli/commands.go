package cli

import (
	"bytes"
	"fmt"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
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
