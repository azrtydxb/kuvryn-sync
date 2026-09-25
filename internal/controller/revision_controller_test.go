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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

var _ = Describe("Revision Controller", func() {
	const resourceName = "payments-8c51af2"

	ctx := context.Background()
	typeNamespacedName := types.NamespacedName{Name: resourceName, Namespace: "default"}

	AfterEach(func() {
		resource := &corev1alpha1.Revision{}
		if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		}
	})

	It("initializes revision lifecycle phase", func() {
		resource := &corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: corev1alpha1.RevisionSpec{
				ApplicationRef: corev1alpha1.LocalObjectReference{Name: "payments"},
				Source: corev1alpha1.RevisionSource{
					RepositoryRef: corev1alpha1.LocalObjectReference{Name: "platform"},
					Revision:      "8c51af2",
					Path:          "apps/payments",
					Render:        corev1alpha1.RenderSpec{Type: corev1alpha1.RenderTypeKustomize},
				},
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())

		controllerReconciler := &RevisionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
		Expect(err).NotTo(HaveOccurred())

		updated := &corev1alpha1.Revision{}
		Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
		Expect(updated.Status.Phase).To(Equal(corev1alpha1.RevisionPhasePending))
	})
})
