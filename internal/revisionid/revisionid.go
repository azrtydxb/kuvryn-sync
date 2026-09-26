// Package revisionid names the Revision an Application gets for a source
// revision, so the controller and the CLI agree on it.
package revisionid

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// For returns the name and identity hash of the Revision of application for
// the source revision, applied as serviceAccount. The identity covers what
// makes a deployment attempt: the repository, source revision, path, render
// settings and service account. It includes the service account because
// who applies is part of a deployment attempt: switching to an account with
// the right permissions must start a fresh Revision instead of reusing one
// blocked by retry limits.
func For(application *corev1alpha1.Application, revision, serviceAccount string) (name, hash string, err error) {
	raw, err := json.Marshal(struct {
		Application    string                            `json:"application"`
		Repository     corev1alpha1.LocalObjectReference `json:"repository"`
		Revision       string                            `json:"revision"`
		Path           string                            `json:"path"`
		Render         corev1alpha1.RenderSpec           `json:"render"`
		ServiceAccount string                            `json:"serviceAccount"`
	}{
		Application:    application.Name,
		Repository:     application.Spec.Source.RepositoryRef,
		Revision:       revision,
		Path:           application.Spec.Source.Path,
		Render:         application.Spec.Source.Render,
		ServiceAccount: serviceAccount,
	})
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	hash = hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s-%s", application.Name, hash[:12]), hash, nil
}

// RollbackBinding is what a manual rollback to rev approves, recorded by the
// admission webhook when the rollback is requested: the desired state rev's
// last completed rollout deployed, applied under application's sync policy
// and strategy as they are then. It is empty for a Revision no rollout
// deployed, which a rollback therefore never approves.
func RollbackBinding(rev *corev1alpha1.Revision, application *corev1alpha1.Application) string {
	return Binding(rev.Status.DeployedDesiredStateHash, application)
}

// Binding fingerprints a rollback target's desired state together with
// application's sync policy and rollout strategy, which decide how it is
// applied: prune, conflict policy, self-heal, and the rollout and failure
// policy. The controller approves a manual rollback only while the Binding
// of the target's current render under the current spec equals the one
// recorded at request time. It is empty without a desired state.
func Binding(desiredStateHash string, application *corev1alpha1.Application) string {
	if desiredStateHash == "" {
		return ""
	}
	raw, err := json.Marshal(struct {
		DesiredStateHash string                          `json:"desiredStateHash"`
		Sync             corev1alpha1.SyncPolicy         `json:"sync"`
		Strategy         corev1alpha1.DeploymentStrategy `json:"strategy"`
	}{desiredStateHash, application.Spec.Sync, application.Spec.Strategy})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
