/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"sync"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/applier"
)

// driftWatches starts metadata-only watches for managed kinds the controller
// is allowed to list and watch. Kinds it may not watch are left to periodic
// resync, and their permission is re-checked at most once per recheck period.
type driftWatches struct {
	controller controller.Controller
	cache      cache.Cache
	client     client.Client
	recheck    time.Duration

	// mu guards the maps only; permission checks and watch registration run
	// unlocked so one slow kind does not hold up every worker.
	mu      sync.Mutex
	watched map[schema.GroupKind]bool
	denied  map[schema.GroupKind]time.Time
	// pending holds a channel per kind being checked, closed once the check
	// ends, so concurrent callers wait instead of registering it twice.
	pending map[schema.GroupKind]chan struct{}
}

func newDriftWatches(ctrl controller.Controller, informers cache.Cache, c client.Client, recheck time.Duration) *driftWatches {
	watched := map[schema.GroupKind]bool{}
	// The default kinds are watched from startup; the controller role grants
	// list/watch on them.
	for _, gvk := range applier.DefaultKinds {
		watched[gvk.GroupKind()] = true
	}
	return &driftWatches{
		controller: ctrl, cache: informers, client: c, recheck: recheck,
		watched: watched, denied: map[schema.GroupKind]time.Time{}, pending: map[schema.GroupKind]chan struct{}{},
	}
}

// ensure watches every kind it can and reports whether all kinds are watched.
func (w *driftWatches) ensure(ctx context.Context, kinds []schema.GroupVersionKind) bool {
	all := true
	for _, gvk := range kinds {
		if !w.ensureKind(ctx, gvk) {
			all = false
		}
	}
	return all
}

func (w *driftWatches) ensureKind(ctx context.Context, gvk schema.GroupVersionKind) bool {
	gk := gvk.GroupKind()
	for {
		w.mu.Lock()
		if w.watched[gk] {
			w.mu.Unlock()
			return true
		}
		if deniedAt, ok := w.denied[gk]; ok && time.Since(deniedAt) < w.recheck {
			w.mu.Unlock()
			return false
		}
		inFlight, busy := w.pending[gk]
		if !busy {
			done := make(chan struct{})
			w.pending[gk] = done
			w.mu.Unlock()

			watched := w.register(ctx, gvk)

			w.mu.Lock()
			if watched {
				delete(w.denied, gk)
				w.watched[gk] = true
			} else {
				w.denied[gk] = time.Now()
			}
			delete(w.pending, gk)
			close(done)
			w.mu.Unlock()
			return watched
		}
		w.mu.Unlock()
		// Another worker is checking this kind; use its outcome.
		select {
		case <-inFlight:
		case <-ctx.Done():
			return false
		}
	}
}

// register checks that the controller may list and watch gvk and starts a
// metadata watch on it, reporting whether the kind is now watched.
func (w *driftWatches) register(ctx context.Context, gvk schema.GroupVersionKind) bool {
	gk := gvk.GroupKind()
	log := logf.FromContext(ctx)
	mapping, err := w.client.RESTMapper().RESTMapping(gk, gvk.Version)
	if err != nil {
		return false
	}
	for _, verb := range []string{"list", "watch"} {
		review := &authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{Group: gk.Group, Resource: mapping.Resource.Resource, Verb: verb},
		}}
		if err := w.client.Create(ctx, review); err != nil || !review.Status.Allowed {
			log.V(1).Info("Controller may not watch managed kind; relying on resync", "kind", gk.String(), "verb", verb)
			return false
		}
	}
	watched := &metav1.PartialObjectMetadata{}
	watched.SetGroupVersionKind(gvk)
	managed := predicate.NewTypedPredicateFuncs(func(obj *metav1.PartialObjectMetadata) bool {
		return obj.GetLabels()[applier.ApplicationLabelKey] != ""
	})
	enqueue := handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, obj *metav1.PartialObjectMetadata) []reconcile.Request {
		return managedObjectToApplication(ctx, obj)
	})
	if err := w.controller.Watch(source.Kind(w.cache, watched, enqueue, managed)); err != nil {
		log.Error(err, "Failed to watch managed kind", "kind", gk.String())
		return false
	}
	log.Info("Watching managed kind for drift", "kind", gk.String())
	return true
}

// objectKinds returns the distinct kinds of objects, sorted for stable status.
func objectKinds(objects []unstructured.Unstructured) []schema.GroupVersionKind {
	kinds := make([]schema.GroupVersionKind, 0, len(objects))
	for _, obj := range objects {
		kinds = append(kinds, obj.GroupVersionKind())
	}
	return applier.UnionKinds(kinds)
}

func managedKinds(kinds []schema.GroupVersionKind) []corev1alpha1.ManagedKind {
	out := make([]corev1alpha1.ManagedKind, 0, len(kinds))
	for _, gvk := range kinds {
		apiVersion, kind := gvk.ToAPIVersionAndKind()
		out = append(out, corev1alpha1.ManagedKind{APIVersion: apiVersion, Kind: kind})
	}
	return out
}
