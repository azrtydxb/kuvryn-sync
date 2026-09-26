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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

var _ = Describe("Adopting fields owned by another manager", func() {
	const appName = "adopting-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}
	liveKey := types.NamespacedName{Name: "app-config", Namespace: "payments"}

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		createRepository(ctx)
		foreign := configMapObject("payments", "theirs")
		Expect(k8sClient.Apply(ctx, client.ApplyConfigurationFromUnstructured(&foreign), client.FieldOwner("argocd-application-controller"))).To(Succeed())
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		deleteApplicationRevisions(ctx, appName)
	})

	It("lists the takeover in the plan, waits for approval, then owns the field", func() {
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyAdopt
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))
		Expect(revision.Status.Plan.Resources).To(HaveLen(1))
		Expect(revision.Status.Plan.Resources[0].Conflicts).To(ContainElement(corev1alpha1.PlanConflict{
			Path: "data.key", Manager: "argocd-application-controller", Policy: corev1alpha1.ConflictPolicyAdopt,
		}))

		approve(ctx, key, revision, "alice@example.com")
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, liveKey, live)).To(Succeed())
		Expect(live.Data).To(HaveKeyWithValue("key", "desired"))
		for _, managed := range live.ManagedFields {
			if managed.Manager == "argocd-application-controller" {
				Expect(string(managed.FieldsV1.Raw)).NotTo(ContainSubstring(`"f:key"`), "the previous manager still owns data.key")
			}
		}
	})

	// Catches an installation migrated to Kuvryn Sync from client-side apply
	// being unable to change any field it had then: the API server records
	// those fields under before-first-apply, and every change conflicted.
	It("takes over fields only the legacy before-first-apply owner holds", func() {
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		legacy := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}, Data: map[string]string{"key": "before"}}
		Expect(k8sClient.Create(ctx, legacy, client.FieldOwner("before-first-apply"))).To(Succeed())
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure).To(BeNil())
		Expect(revision.Status.Plan.Resources[0].Conflicts).To(ContainElement(corev1alpha1.PlanConflict{
			Path: "data.key", Manager: "before-first-apply", Policy: corev1alpha1.ConflictPolicyAdopt,
		}))
		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, liveKey, live)).To(Succeed())
		Expect(live.Data).To(HaveKeyWithValue("key", "desired"))
		for _, managed := range live.ManagedFields {
			if managed.Manager == "before-first-apply" {
				Expect(string(managed.FieldsV1.Raw)).NotTo(ContainSubstring(`"f:key"`), "the legacy owner still holds data.key")
			}
		}
	})

	It("still fails on the same conflict by default", func() {
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(listApplicationRevisions(ctx, appName).Items[0].Status.Failure.Reason).To(Equal("ConflictFailure"))
		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, liveKey, live)).To(Succeed())
		Expect(live.Data).To(HaveKeyWithValue("key", "theirs"))
	})
})
