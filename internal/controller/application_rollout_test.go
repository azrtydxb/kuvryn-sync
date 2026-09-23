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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/health"
	"github.com/azrtydxb/solder/internal/planner"
)

func TestGroupHealthCountsMissingObjects(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
	missing := configMapObject("payments", "desired")

	results, err := groupHealth(context.Background(), c, health.Evaluator{}, []unstructured.Unstructured{missing}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].State != corev1alpha1.HealthStateProgressing {
		t.Fatalf("missing sync object = %#v, want one Progressing result", results)
	}

	results, err = groupHealth(context.Background(), c, health.Evaluator{}, []unstructured.Unstructured{missing}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].State != corev1alpha1.HealthStateDegraded {
		t.Fatalf("missing hook = %#v, want one Degraded result", results)
	}
}

func TestHealthCheckCacheCompilesOnlyWhenHealthChecksChange(t *testing.T) {
	check := func(resourceVersion, expression string) corev1alpha1.HealthCheck {
		return corev1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "widgets", UID: "uid-1", ResourceVersion: resourceVersion},
			Spec: corev1alpha1.HealthCheckSpec{Group: "example.com", Kind: "Widget", Rules: []corev1alpha1.HealthRule{{
				Expression: expression, State: corev1alpha1.HealthStateProgressing, Message: "matched",
			}}},
		}
	}
	widget := customObject("Widget", "w", "")
	widget.SetNamespace("payments")
	state := func(evaluator health.Evaluator) corev1alpha1.HealthState {
		t.Helper()
		result, err := evaluator.Evaluate(widget)
		if err != nil {
			t.Fatal(err)
		}
		return result.State
	}
	cache := &healthCheckCache{}

	first, err := cache.get([]corev1alpha1.HealthCheck{check("1", "true")})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := cache.get([]corev1alpha1.HealthCheck{check("1", "true")})
	if cache.compiles != 1 || state(first) != corev1alpha1.HealthStateProgressing || state(again) != corev1alpha1.HealthStateProgressing {
		t.Fatalf("compiles = %d, want 1 for unchanged HealthChecks", cache.compiles)
	}

	changed, _ := cache.get([]corev1alpha1.HealthCheck{check("2", "false")})
	if cache.compiles != 2 || state(changed) != corev1alpha1.HealthStateHealthy {
		t.Fatalf("compiles = %d, want a recompile that applies the changed rule", cache.compiles)
	}
	removed, _ := cache.get(nil)
	if cache.compiles != 3 || state(removed) != corev1alpha1.HealthStateHealthy {
		t.Fatalf("compiles = %d, want a recompile once the HealthCheck is gone", cache.compiles)
	}
}

func TestPlanDigestHashesTheRedactedPlan(t *testing.T) {
	// spec.value is a path the planner does not already treat as sensitive,
	// so only the Helm value masking hides it.
	greeting := func(value string) unstructured.Unstructured {
		obj := customObject("Widget", "greeting", value)
		obj.SetNamespace("payments")
		return obj
	}
	live := greeting("hello")
	digest := func(desiredHash, secret string) string {
		t.Helper()
		plan, err := planner.Build([]unstructured.Unstructured{greeting(secret)}, []unstructured.Unstructured{live})
		if err != nil {
			t.Fatal(err)
		}
		out, err := planDigest(desiredHash, plan, []string{secret})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// With the desired state fixed, only the secret value differs, and it is
	// masked before hashing: it must not feed the digest.
	if digest("desired-1", "s3cret-one") != digest("desired-1", "s3cret-two") {
		t.Fatal("a Secret-sourced Helm value fed the plan digest in clear")
	}
	// A real value change changes the rendered desired state, and with it the
	// digest.
	if digest("desired-1", "s3cret-one") == digest("desired-2", "s3cret-two") {
		t.Fatal("the digest ignored a change to the desired state")
	}
}
