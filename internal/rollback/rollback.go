package rollback

import (
	"errors"
	"fmt"
	"sort"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// ErrNoTarget reports that an Application has no earlier healthy Revision to
// roll back to, such as on its first rollout. Callers tell it apart from a
// failure to look Revisions up with errors.Is.
var ErrNoTarget = errors.New("no previous healthy Revision")

// Target returns the most recent healthy Revision before current for the same
// Application, or an error wrapping ErrNoTarget when there is none.
func Target(current corev1alpha1.Revision, history []corev1alpha1.Revision) (corev1alpha1.Revision, error) {
	candidates := []corev1alpha1.Revision{}
	for _, rev := range history {
		if rev.Name == current.Name || rev.Spec.ApplicationRef.Name != current.Spec.ApplicationRef.Name {
			continue
		}
		if rev.Status.Phase != corev1alpha1.RevisionPhaseHealthy {
			continue
		}
		if !current.CreationTimestamp.IsZero() && !rev.CreationTimestamp.Before(&current.CreationTimestamp) {
			continue
		}
		candidates = append(candidates, rev)
	}
	if len(candidates) == 0 {
		return corev1alpha1.Revision{}, fmt.Errorf("%w found for Application %q", ErrNoTarget, current.Spec.ApplicationRef.Name)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i].CreationTimestamp, candidates[j].CreationTimestamp
		if !a.Equal(&b) {
			return a.After(b.Time)
		}
		return candidates[i].Name > candidates[j].Name
	})
	return candidates[0], nil
}
