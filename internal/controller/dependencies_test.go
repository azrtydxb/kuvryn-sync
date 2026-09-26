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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

var _ = Describe("Application dependencies", func() {
	ctx := context.Background()
	workload := types.NamespacedName{Name: "workload", Namespace: "default"}
	configKey := types.NamespacedName{Name: "app-config", Namespace: "payments"}

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		createRepository(ctx)
	})

	AfterEach(func() {
		for _, name := range []string{"workload", "operator", "a", "b"} {
			deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})
			deleteApplicationRevisions(ctx, name)
		}
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
	})

	create := func(name string, dependsOn ...string) {
		app := newApplication(name, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		for _, dep := range dependsOn {
			app.Spec.DependsOn = append(app.Spec.DependsOn, corev1alpha1.LocalObjectReference{Name: dep})
		}
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
	}
	condition := func(key types.NamespacedName) *metav1.Condition {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		return apimeta.FindStatusCondition(app.Status.Conditions, "DependenciesReady")
	}

	It("waits for a dependency to be Healthy at its desired revision, then applies", func() {
		create("operator")
		create("workload", "operator")
		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workload})
		Expect(err).NotTo(HaveOccurred())
		Expect(condition(workload).Reason).To(Equal("DependencyNotReady"))
		Expect(condition(workload).Message).To(ContainSubstring("operator"))
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue(), "applied before its dependency was Healthy")
		// Waiting for a dependency does not count against the health
		// timeout of the rollout that follows.
		Expect(listApplicationRevisions(ctx, "workload").Items[0].Status.StartedAt).To(BeNil())

		operator := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "operator", Namespace: "default"}, operator)).To(Succeed())
		operator.Status.ObservedGeneration = operator.Generation
		operator.Status.Health.State = corev1alpha1.HealthStateHealthy
		operator.Status.DesiredRevision, operator.Status.DeployedRevision = "sha-2", "sha-1"
		Expect(k8sClient.Status().Update(ctx, operator)).To(Succeed())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workload})
		Expect(err).NotTo(HaveOccurred())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue(), "applied while its dependency was still rolling out a new revision")

		operator.Status.DeployedRevision = "sha-2"
		Expect(k8sClient.Status().Update(ctx, operator)).To(Succeed())
		Expect(reconciler.dependentsOf(ctx, operator)).To(ConsistOf(reconcile.Request{NamespacedName: workload}))

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: workload})
		Expect(err).NotTo(HaveOccurred())
		Expect(condition(workload).Status).To(Equal(metav1.ConditionTrue))
		Expect(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{})).To(Succeed())
	})

	It("reports a dependency cycle instead of waiting forever", func() {
		create("a", "b")
		create("b", "a")
		reconciler := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "a", Namespace: "default"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
		got := condition(types.NamespacedName{Name: "a", Namespace: "default"})
		Expect(got.Reason).To(Equal("DependencyCycle"))
		Expect(got.Message).To(ContainSubstring("a -> b -> a"))
	})
})
