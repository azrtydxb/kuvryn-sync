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
