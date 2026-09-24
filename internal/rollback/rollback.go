package rollback

import (
	"fmt"
	"sort"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

// Target returns the most recent healthy Revision before current for the same Application.
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
		return corev1alpha1.Revision{}, fmt.Errorf("no previous healthy Revision found for Application %q", current.Spec.ApplicationRef.Name)
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
