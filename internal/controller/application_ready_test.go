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
	"errors"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

var _ = Describe("Application Ready condition", func() {
	const name = "ready-application"
	ctx := context.Background()
	key := types.NamespacedName{Name: name, Namespace: "default"}

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		deleteApplicationRevisions(ctx, name)
	})

	ready := func() *metav1.Condition {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		condition := apimeta.FindStatusCondition(app.Status.Conditions, ReadyCondition)
		Expect(condition).NotTo(BeNil())
		return condition
	}

	// Catches a Ready condition that is only ever set to False, which left a
	// recovered Application reporting a stale failure.
	It("becomes True once a failed Application recovers and False again on a new failure", func() {
		application := newApplication(name, corev1alpha1.RenderTypeYAML)
		application.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, application)).To(Succeed())
		capture := &capturingRenderer{objects: []unstructured.Unstructured{configMapObject("", "desired")}}
		reconciler := newApplicationReconciler(nil, capture)

		By("failing while the Repository is missing")
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		failed := ready()
		Expect(failed.Status).To(Equal(metav1.ConditionFalse))
		Expect(failed.Reason).To(Equal("SourceFailure"))

		By("recovering once the Repository exists")
		createRepository(ctx)
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		recovered := ready()
		Expect(recovered.Status).To(Equal(metav1.ConditionTrue))
		Expect(recovered.Reason).To(Equal("Healthy"))
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		Expect(recovered.ObservedGeneration).To(Equal(app.Generation))
		Expect(app.Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))
		Expect(app.Status.Health.State).To(Equal(corev1alpha1.HealthStateHealthy))

		By("keeping the condition untouched on a steady-state reconcile")
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		steady := ready()
		Expect(steady.Status).To(Equal(metav1.ConditionTrue))
		Expect(steady.LastTransitionTime).To(Equal(recovered.LastTransitionTime))
		Expect(steady.Message).To(Equal(recovered.Message))

		By("turning False when a new Revision fails")
		capture.err = errors.New("render failed")
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		app.Spec.Source.Path = "apps/payments-v2"
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		refailed := ready()
		Expect(refailed.Status).To(Equal(metav1.ConditionFalse))
		Expect(refailed.Reason).To(Equal("RenderFailure"))
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		Expect(refailed.ObservedGeneration).To(Equal(app.Generation))
	})
})
