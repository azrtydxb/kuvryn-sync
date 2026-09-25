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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

func TestUpsertDiscoveredApplicationRefusesAnApplicationAnotherControllerOwns(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	owned := &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{
		Name: "payments", Namespace: "default",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: corev1alpha1.GroupVersion.String(), Kind: "Repository", Name: "other", UID: "other-uid", Controller: ptr.To(true)}},
	}, Spec: corev1alpha1.ApplicationSpec{Source: corev1alpha1.ApplicationSource{Path: "kept"}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).Build()
	r := &RepositoryReconciler{Client: c, Scheme: scheme}
	repository := &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default", UID: "platform-uid"}}
	desired := &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"}, Spec: corev1alpha1.ApplicationSpec{Source: corev1alpha1.ApplicationSource{Path: "taken"}}}

	if err := r.upsertDiscoveredApplication(context.Background(), repository, desired, solderConfigFileName); err == nil {
		t.Fatal("took over an Application another controller owns")
	}
	got := &corev1alpha1.Application{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(owned), got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Source.Path != "kept" {
		t.Fatalf("spec was overwritten: %q", got.Spec.Source.Path)
	}
}

// Catches a .solder.yaml starting a rollback: rollbacks come from people or
// a failure policy, never from Git.
func TestDiscoveredApplicationsCannotRequestARollback(t *testing.T) {
	repository := &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}}
	app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments", Annotations: map[string]string{
		corev1alpha1.RollbackRevisionAnnotation: "a-sha",
		corev1alpha1.RollbackFromAnnotation:     "b-sha",
		corev1alpha1.RollbackKindAnnotation:     corev1alpha1.RollbackKindManual,
		"team":                                  "payments",
	}}}
	app.Spec.Source.Render.Type = corev1alpha1.RenderTypeYAML
	normalized, err := normalizeDiscoveredApplication(repository, solderConfigFileName, 0, app, map[string]struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{corev1alpha1.RollbackRevisionAnnotation, corev1alpha1.RollbackFromAnnotation, corev1alpha1.RollbackKindAnnotation} {
		if _, ok := normalized.Annotations[key]; ok {
			t.Errorf("discovery kept %s", key)
		}
	}
	if normalized.Annotations["team"] != "payments" {
		t.Errorf("discovery dropped an ordinary annotation: %v", normalized.Annotations)
	}
}
