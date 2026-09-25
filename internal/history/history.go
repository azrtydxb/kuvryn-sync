package history

import (
	"sort"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// Retention is the result of applying an Application history limit.
type Retention struct {
	Keep   []corev1alpha1.Revision
	Delete []corev1alpha1.Revision
}

// ApplyRetention keeps the newest limit Revisions and returns older entries for GC.
func ApplyRetention(revisions []corev1alpha1.Revision, limit int32) Retention {
	ordered := append([]corev1alpha1.Revision{}, revisions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i].CreationTimestamp, ordered[j].CreationTimestamp
		if !a.Equal(&b) {
			return a.After(b.Time)
		}
		return ordered[i].Name > ordered[j].Name
	})
	if limit <= 0 || int(limit) >= len(ordered) {
		return Retention{Keep: ordered}
	}
	keep := append([]corev1alpha1.Revision{}, ordered[:limit]...)
	delete := append([]corev1alpha1.Revision{}, ordered[limit:]...)
	return Retention{Keep: keep, Delete: delete}
}

// LimitFor returns the effective bounded history limit, defaulting conservatively.
func LimitFor(app corev1alpha1.Application, fallback int32) int32 {
	if fallback <= 0 {
		fallback = 20
	}
	if app.Spec.History.Limit == nil || *app.Spec.History.Limit <= 0 {
		return fallback
	}
	return *app.Spec.History.Limit
}
