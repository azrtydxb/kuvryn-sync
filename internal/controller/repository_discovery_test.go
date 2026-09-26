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

	if err := r.upsertDiscoveredApplication(context.Background(), repository, desired, configFileName); err == nil {
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

// Catches a .ksync.yaml starting a rollback: rollbacks come from people or
// a failure policy, never from Git.
func TestDiscoveredApplicationsCannotRequestARollback(t *testing.T) {
	repository := &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}}
	app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments", Annotations: map[string]string{
		corev1alpha1.RollbackRevisionAnnotation: "a-sha",
		corev1alpha1.RollbackFromAnnotation:     "b-sha",
		corev1alpha1.RollbackKindAnnotation:     corev1alpha1.RollbackKindManual,
		// A request's record would otherwise make the webhook refuse the
		// Application, or claim a requester Git cannot name.
		corev1alpha1.RollbackTargetRevisionAnnotation: "payments-abc",
		corev1alpha1.RollbackTargetHashAnnotation:     "hash",
		corev1alpha1.RollbackRequestedByAnnotation:    "mallory@example.com",
		corev1alpha1.RollbackRequestedAtAnnotation:    "2026-09-26T10:00:00Z",
		"team": "payments",
	}}}
	app.Spec.Source.Render.Type = corev1alpha1.RenderTypeYAML
	normalized, err := normalizeDiscoveredApplication(repository, configFileName, 0, app, map[string]struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		corev1alpha1.RollbackRevisionAnnotation, corev1alpha1.RollbackFromAnnotation, corev1alpha1.RollbackKindAnnotation,
		corev1alpha1.RollbackTargetRevisionAnnotation, corev1alpha1.RollbackTargetHashAnnotation,
		corev1alpha1.RollbackRequestedByAnnotation, corev1alpha1.RollbackRequestedAtAnnotation,
	} {
		if _, ok := normalized.Annotations[key]; ok {
			t.Errorf("discovery kept %s", key)
		}
	}
	if normalized.Annotations["team"] != "payments" {
		t.Errorf("discovery dropped an ordinary annotation: %v", normalized.Annotations)
	}
}

// Catches Git switching off the approval gate: what a discovered Application
// may do without a person's approval is the Repository owner's decision.
func TestDiscoveredApplicationsStayWithinTheRepositoryApplicationPolicy(t *testing.T) {
	automatic := func(app *corev1alpha1.Application) { app.Spec.Sync.Automatic = true }
	prune := func(app *corev1alpha1.Application) { app.Spec.Sync.Prune = true }
	adopt := func(app *corev1alpha1.Application) { app.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyAdopt }
	deleteManaged := func(app *corev1alpha1.Application) {
		app.Spec.DeletionPolicy = corev1alpha1.DeletionPolicyDeleteManagedResources
	}
	cases := []struct {
		name      string
		policy    corev1alpha1.ApplicationPolicy
		set       func(*corev1alpha1.Application)
		wantError string
	}{
		{name: "manual sync needs nothing", set: func(*corev1alpha1.Application) {}},
		{name: "self-heal needs nothing", set: func(app *corev1alpha1.Application) { app.Spec.Sync.SelfHeal = true }},
		{name: "fail and orphan need nothing", set: func(app *corev1alpha1.Application) {
			app.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyFail
			app.Spec.DeletionPolicy = corev1alpha1.DeletionPolicyOrphan
		}},
		{name: "automatic refused by default", set: automatic, wantError: "spec.applicationPolicy.allowAutomatic"},
		{name: "prune refused by default", set: prune, wantError: "spec.applicationPolicy.allowPrune"},
		{name: "adopt refused by default", set: adopt, wantError: "spec.applicationPolicy.allowAdopt"},
		{name: "deleting managed resources refused by default", set: deleteManaged, wantError: "spec.applicationPolicy.allowDeleteManagedResources"},
		{name: "automatic allowed", policy: corev1alpha1.ApplicationPolicy{AllowAutomatic: true}, set: automatic},
		{name: "prune allowed", policy: corev1alpha1.ApplicationPolicy{AllowPrune: true}, set: prune},
		{name: "adopt allowed", policy: corev1alpha1.ApplicationPolicy{AllowAdopt: true}, set: adopt},
		{name: "deleting managed resources allowed", policy: corev1alpha1.ApplicationPolicy{AllowDeleteManagedResources: true}, set: deleteManaged},
		{name: "one allowance does not grant another", policy: corev1alpha1.ApplicationPolicy{AllowPrune: true}, set: automatic, wantError: "allowAutomatic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository := &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}}
			repository.Spec.ApplicationPolicy = tc.policy
			app := corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments"}}
			app.Spec.Source.Render.Type = corev1alpha1.RenderTypeYAML
			tc.set(&app)
			_, err := normalizeDiscoveredApplication(repository, configFileName, 0, app, map[string]struct{}{})
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("refused an allowed Application: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("got error %v, want one naming %q", err, tc.wantError)
			}
			if !strings.Contains(err.Error(), `"payments"`) || !strings.Contains(err.Error(), `"platform"`) {
				t.Errorf("error does not name the Application and Repository: %v", err)
			}
		})
	}
}
