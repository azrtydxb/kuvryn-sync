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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
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

	It("fails with a retryable validation error for kinds the cluster does not know", func() {
		widget := unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "example.com/v1",
			"kind":       "Widget",
			"metadata":   map[string]any{"name": "gear"},
		}}
		reconciler := newApplicationReconciler([]unstructured.Unstructured{widget}, nil)
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		revision := listApplicationRevisions(ctx, appName).Items[0]
		Expect(revision.Status.Failure).NotTo(BeNil())
		Expect(revision.Status.Failure.Reason).To(Equal("ValidationFailure"))
		Expect(revision.Status.Failure.Retryable).To(BeTrue())
		Expect(revision.Status.Failure.Message).To(ContainSubstring("Widget"))
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
