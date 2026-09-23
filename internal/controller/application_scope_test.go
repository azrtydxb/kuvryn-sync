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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

var _ = Describe("Application destination namespace", func() {
	const appName = "scope-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		createRepository(ctx)
		Expect(k8sClient.Create(ctx, newApplication(appName, corev1alpha1.RenderTypeYAML))).To(Succeed())
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		deleteApplicationRevisions(ctx, appName)
	})

	It("defaults only namespaced objects to the destination, using the cluster's scope", func() {
		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired"), storageClass("fast")}, nil)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure).To(BeNil())
		namespaces := map[string]string{}
		for _, planned := range revision.Status.Plan.Resources {
			namespaces[planned.Resource.Kind] = planned.Resource.Namespace
		}
		Expect(namespaces).To(Equal(map[string]string{"ConfigMap": "payments", "StorageClass": ""}))
	})

	It("completes a status-less custom resource as Healthy without waiting for the health timeout", func() {
		ensureCustomKind(ctx, "Widget", "widgets")
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		app.Spec.Sync.Automatic = true
		app.Spec.Health.Timeout = &metav1.Duration{Duration: time.Millisecond}
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
		DeferCleanup(func() {
			widget := customObject("Widget", "gear", "v1")
			widget.SetNamespace("payments")
			_ = k8sClient.Delete(ctx, &widget)
		})

		reconciler := newApplicationReconciler([]unstructured.Unstructured{customObject("Widget", "gear", "v1")}, nil)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure).To(BeNil())
		Expect(revision.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		Expect(app.Status.Health.State).To(Equal(corev1alpha1.HealthStateHealthy))
	})

	It("prunes a custom-kind object removed from desired state using the inventory", func() {
		ensureCustomKind(ctx, "Widget", "widgets")
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
		widget := customObject("Widget", "pruned-widget", "v1")
		widgetKey := client.ObjectKey{Name: "pruned-widget", Namespace: "payments"}
		DeferCleanup(func() {
			stale := customObject("Widget", "pruned-widget", "")
			stale.SetNamespace("payments")
			_ = k8sClient.Delete(ctx, &stale)
		})

		capture := &capturingRenderer{objects: []unstructured.Unstructured{widget}}
		reconciler := newApplicationReconciler(nil, capture)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		live := customObject("Widget", "", "")
		Expect(k8sClient.Get(ctx, widgetKey, &live)).To(Succeed())
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		Expect(app.Status.ManagedKinds).To(ContainElement(corev1alpha1.ManagedKind{APIVersion: "example.com/v1", Kind: "Widget"}))

		capture.objects = []unstructured.Unstructured{configMapObject("", "desired")}
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, widgetKey, &live))).To(BeTrue(), "stale Widget was not pruned")
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		Expect(app.Status.ManagedKinds).To(Equal([]corev1alpha1.ManagedKind{{APIVersion: "v1", Kind: "ConfigMap"}}))
	})

	It("fails with a retryable validation error for kinds the cluster does not know", func() {
		unknown := unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "unknown.example.com/v1",
			"kind":       "Gadget",
			"metadata":   map[string]any{"name": "gear"},
		}}
		reconciler := newApplicationReconciler([]unstructured.Unstructured{unknown}, nil)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure).NotTo(BeNil())
		Expect(revision.Status.Failure.Reason).To(Equal("ValidationFailure"))
		Expect(revision.Status.Failure.Retryable).To(BeTrue())
		Expect(revision.Status.Failure.Message).To(ContainSubstring("Gadget"))
	})
})

func storageClass(name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{
		"apiVersion":  "storage.k8s.io/v1",
		"kind":        "StorageClass",
		"metadata":    map[string]any{"name": name},
		"provisioner": "example.com/fast",
	}}
}
