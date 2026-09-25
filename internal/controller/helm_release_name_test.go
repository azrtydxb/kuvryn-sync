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
	"strings"
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

// TestInvalidHelmReleaseNameFailsBeforeAnyRevision proves an Application
// stored before the CRD refused invalid release names reports a
// ValidationFailure once, instead of retrying a Revision Create the Revision
// CRD rejects. The fake client stands in for such an Application because it
// does not apply CRD validation. Proved by: removing the check makes Reconcile
// go on to the service account and never record ValidationFailure.
func TestInvalidHelmReleaseNameFailsBeforeAnyRevision(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	app := newApplication("legacy", corev1alpha1.RenderTypeHelm)
	app.Spec.Source.Render.Helm = &corev1alpha1.HelmRenderSpec{ReleaseName: "Payments_Prod"}
	app.Spec.ServiceAccountName = "deployer"
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).
		WithStatusSubresource(&corev1alpha1.Application{}).Build()
	r := &ApplicationReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}
	ctx := context.Background()

	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(app)})
	if err != nil || !result.IsZero() {
		t.Fatalf("an invalid release name must not be retried: result %+v, err %v", result, err)
	}
	got := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(app), got); err != nil {
		t.Fatal(err)
	}
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Reason != "ValidationFailure" || !strings.Contains(ready.Message, `"Payments_Prod"`) {
		t.Fatalf("Ready = %+v", ready)
	}
	if got.Status.Health.State != corev1alpha1.HealthStateDegraded {
		t.Fatalf("health = %q", got.Status.Health.State)
	}
	revisions := &corev1alpha1.RevisionList{}
	if err := c.List(ctx, revisions); err != nil {
		t.Fatal(err)
	}
	if len(revisions.Items) != 0 {
		t.Fatalf("created Revisions %v", revisions.Items)
	}
}

func TestValidateHelmReleaseName(t *testing.T) {
	for name, want := range map[string]bool{
		"":                       true, // the default, solder
		"payments.v2":            true,
		strings.Repeat("a", 53):  true,
		strings.Repeat("a", 54):  false,
		"Payments":               false,
		"pay_ments":              false,
		"payments-":              false,
		"--post-renderer=/bin/x": false,
	} {
		app := newApplication("api", corev1alpha1.RenderTypeHelm)
		app.Spec.Source.Render.Helm = &corev1alpha1.HelmRenderSpec{ReleaseName: name}
		if err := validateHelmReleaseName(app); (err == nil) != want {
			t.Errorf("%q: err = %v", name, err)
		}
	}
	if err := validateHelmReleaseName(newApplication("api", corev1alpha1.RenderTypeHelm)); err != nil {
		t.Errorf("a Helm Application without a helm block uses the default name: %v", err)
	}
	yaml := newApplication("api", corev1alpha1.RenderTypeYAML)
	yaml.Spec.Source.Render.Helm = &corev1alpha1.HelmRenderSpec{ReleaseName: "Ignored"}
	if err := validateHelmReleaseName(yaml); err != nil {
		t.Errorf("only Helm Applications have a release name: %v", err)
	}
}
