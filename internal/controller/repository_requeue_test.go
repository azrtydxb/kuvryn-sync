package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

func repoAt(rev string, fetched time.Time, mutate func(*corev1alpha1.Repository)) *corev1alpha1.Repository {
	r := &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "n", Generation: 1}}
	r.Status.State = corev1alpha1.RepositoryStateReady
	r.Status.ObservedRevision = rev
	t := metav1.NewTime(fetched)
	r.Status.LastFetchedAt = &t
	if mutate != nil {
		mutate(r)
	}
	return r
}

// A Repository reconcile stamps lastFetchedAt on every pass. If that status
// write woke the Repository controller again it would refetch every few
// seconds and ignore pollInterval.
func TestRepositoryChangesIgnoresItsOwnStatusWrites(t *testing.T) {
	now := time.Now()
	old, updated := repoAt("abc", now, nil), repoAt("abc", now.Add(3*time.Second), nil)
	if repositoryChanges.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: updated}) {
		t.Fatal("a lastFetchedAt-only status write must not requeue the Repository")
	}
	spec := repoAt("abc", now, func(r *corev1alpha1.Repository) { r.Generation = 2 })
	if !repositoryChanges.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: spec}) {
		t.Fatal("a spec change must requeue the Repository")
	}
	annotated := repoAt("abc", now, func(r *corev1alpha1.Repository) {
		r.Annotations = map[string]string{"sync.kuvryn.io/requested-at": "x"}
	})
	if !repositoryChanges.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: annotated}) {
		t.Fatal("a fetch request annotation must requeue the Repository")
	}
}

// Applications follow the commit a Repository observed; a fetch that finds
// nothing new must not make every Application plan against live state again.
func TestApplicationsWatchRepositorySourceChangesOnly(t *testing.T) {
	now := time.Now()
	old := repoAt("abc", now, nil)
	if repositorySourceChanged.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: repoAt("abc", now.Add(3*time.Second), nil)}) {
		t.Fatal("an unchanged observed revision must not requeue Applications")
	}
	if !repositorySourceChanged.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: repoAt("def", now.Add(3*time.Second), nil)}) {
		t.Fatal("a new observed revision must requeue Applications")
	}
	failed := repoAt("abc", now, func(r *corev1alpha1.Repository) { r.Status.State = corev1alpha1.RepositoryStateFailed })
	if !repositorySourceChanged.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: failed}) {
		t.Fatal("a Repository turning Failed must requeue Applications")
	}
}

func TestRepositoryPollIntervalDefaults(t *testing.T) {
	if got := repositoryPollInterval(&corev1alpha1.Repository{}); got != defaultRepositoryPollInterval {
		t.Fatalf("unset pollInterval = %v, want %v", got, defaultRepositoryPollInterval)
	}
	set := &corev1alpha1.Repository{}
	set.Spec.PollInterval = &metav1.Duration{Duration: 90 * time.Second}
	if got := repositoryPollInterval(set); got != 90*time.Second {
		t.Fatalf("pollInterval = %v", got)
	}
}
