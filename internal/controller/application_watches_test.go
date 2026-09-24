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
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/azrtydxb/solder/internal/applier"
)

// countingController records Watch calls and nothing else.
type countingController struct {
	controller.Controller
	mu      sync.Mutex
	watches int
}

func (c *countingController) Watch(source.TypedSource[reconcile.Request]) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watches++
	return nil
}

func (c *countingController) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.watches
}

func TestDriftWatchesCheckKindsConcurrentlyAndRegisterEachOnce(t *testing.T) {
	widget := schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}
	gizmo := schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Gizmo"}
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(widget, meta.RESTScopeNamespace)
	mapper.Add(gizmo, meta.RESTScopeNamespace)

	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithRESTMapper(mapper).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			review := obj.(*authorizationv1.SelfSubjectAccessReview)
			if attrs := review.Spec.ResourceAttributes; attrs.Resource == "widgets" && attrs.Verb == "list" {
				entered <- struct{}{}
				<-release
			}
			review.Status.Allowed = true
			return nil
		},
	}).Build()
	ctrl := &countingController{}
	watches := newDriftWatches(ctrl, nil, c, time.Minute)
	ctx := context.Background()

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- watches.ensure(ctx, []schema.GroupVersionKind{widget})
		}()
	}
	<-entered

	// While Widget's permission check is stuck, other kinds are not held up.
	other := make(chan bool, 1)
	go func() {
		other <- watches.ensure(ctx, []schema.GroupVersionKind{applier.DefaultKinds[0], gizmo})
	}()
	select {
	case ok := <-other:
		if !ok {
			t.Fatal("ensure(ConfigMap, Gizmo) = false, want true")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ensure for other kinds waited for Widget's permission check")
	}

	close(release)
	wg.Wait()
	close(results)
	for ok := range results {
		if !ok {
			t.Fatal("ensure(Widget) = false, want true")
		}
	}
	if len(entered) != 0 {
		t.Fatalf("Widget permission was checked %d more times, want once", len(entered))
	}
	// One watch for Widget and one for Gizmo.
	if got := ctrl.count(); got != 2 {
		t.Fatalf("Watch called %d times, want 2", got)
	}
}
