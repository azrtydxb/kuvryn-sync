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

	appsv1 "k8s.io/api/apps/v1"
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
	"github.com/azrtydxb/solder/internal/source"
)

type revisionResolver struct{ revision *string }

func (r revisionResolver) Resolve(context.Context, source.GitRepository) (source.ResolvedSource, error) {
	return source.ResolvedSource{Revision: *r.revision, CacheDir: "/tmp/solder-workspace"}, nil
}

var _ = Describe("Sync hooks and waves", func() {
	const appName = "staged-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}
	configKey := types.NamespacedName{Name: "app-config", Namespace: "payments"}
	widgetKey := types.NamespacedName{Name: "migrate", Namespace: "payments"}

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		ensureCustomKind(ctx, "Widget", "widgets")
		// Like a Job that has not reported yet, a Widget without status is
		// still running; kstatus decides once it has conditions.
		Expect(k8sClient.Create(ctx, &corev1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "widgets-running"},
			Spec: corev1alpha1.HealthCheckSpec{Group: "example.com", Kind: "Widget", Rules: []corev1alpha1.HealthRule{{
				Expression: "!has(object.status)", State: corev1alpha1.HealthStateProgressing, Message: "not started",
			}}},
		})).To(Succeed())
		createRepository(ctx)
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.HealthCheck{ObjectMeta: metav1.ObjectMeta{Name: "widgets-running"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		deleteObject(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"}})
		widget := customObject("Widget", "migrate", "")
		widget.SetNamespace("payments")
		_ = k8sClient.Delete(ctx, &widget)
		deleteApplicationRevisions(ctx, appName)
	})

	deployment := func(wave string) unstructured.Unstructured {
		obj := unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "api", "annotations": map[string]any{"solder.io/sync-wave": wave}},
			"spec": map[string]any{
				"replicas": int64(1),
				"selector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
				"template": map[string]any{
					"metadata": map[string]any{"labels": map[string]any{"app": "api"}},
					"spec":     map[string]any{"containers": []any{map[string]any{"name": "api", "image": "nginx"}}},
				},
			},
		}}
		return obj
	}
	annotate := func(obj unstructured.Unstructured, key, value string) unstructured.Unstructured {
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[key] = value
		obj.SetAnnotations(annotations)
		return obj
	}
	setWidgetConditions := func(conditions ...map[string]any) {
		widget := customObject("Widget", "", "")
		Expect(k8sClient.Get(ctx, widgetKey, &widget)).To(Succeed())
		list := make([]any, 0, len(conditions))
		for _, c := range conditions {
			list = append(list, c)
		}
		Expect(unstructured.SetNestedSlice(widget.Object, list, "status", "conditions")).To(Succeed())
		Expect(k8sClient.Update(ctx, &widget)).To(Succeed())
	}
	reconcileOnce := func(r *ApplicationReconciler) {
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
	}
	latestRevision := func() corev1alpha1.Revision {
		revisions := listApplicationRevisions(ctx, appName).Items
		Expect(revisions).NotTo(BeEmpty())
		latest := revisions[0]
		for _, rev := range revisions {
			if rev.CreationTimestamp.After(latest.CreationTimestamp.Time) || rev.Name > latest.Name && rev.CreationTimestamp.Equal(&latest.CreationTimestamp) {
				latest = rev
			}
		}
		return latest
	}

	It("keeps observing an unready rollout instead of declaring it Healthy", func() {
		r := newApplicationReconciler([]unstructured.Unstructured{deployment("0")}, nil)
		reconcileOnce(r)
		reconcileOnce(r)
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseObserving))
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		Expect(app.Status.Health.State).To(Equal(corev1alpha1.HealthStateProgressing))
	})

	It("applies a later wave only once the earlier wave is Healthy", func() {
		r := newApplicationReconciler([]unstructured.Unstructured{deployment("0"), annotate(configMapObject("", "desired"), "solder.io/sync-wave", "1")}, nil)
		reconcileOnce(r)
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue(), "wave 1 applied before wave 0 was Healthy")

		live := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "api", Namespace: "payments"}, live)).To(Succeed())
		live.Status.ObservedGeneration, live.Status.Replicas, live.Status.AvailableReplicas, live.Status.ReadyReplicas, live.Status.UpdatedReplicas = live.Generation, 1, 1, 1, 1
		Expect(k8sClient.Status().Update(ctx, live)).To(Succeed())
		reconcileOnce(r)
		Expect(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{})).To(Succeed())
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
	})

	It("runs a pre-sync hook first, records it, and replaces it for the next Revision", func() {
		revision := "sha-1"
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "pre-sync")
		r := newApplicationReconciler([]unstructured.Unstructured{hook, configMapObject("", "desired")}, nil)
		r.SourceResolver = revisionResolver{revision: &revision}
		reconcileOnce(r)
		setWidgetConditions(map[string]any{"type": "Reconciling", "status": "True", "message": "migrating"})
		reconcileOnce(r)
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue(), "sync objects applied while the pre-sync hook was running")
		Expect(latestRevision().Status.Hooks).To(ConsistOf(HaveField("State", corev1alpha1.HealthStateProgressing)))

		setWidgetConditions(map[string]any{"type": "Ready", "status": "True"})
		reconcileOnce(r)
		Expect(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{})).To(Succeed())
		first := latestRevision()
		Expect(first.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(first.Status.Hooks).To(ConsistOf(And(HaveField("Stage", "PreSync"), HaveField("State", corev1alpha1.HealthStateHealthy))))
		old := customObject("Widget", "", "")
		Expect(k8sClient.Get(ctx, widgetKey, &old)).To(Succeed())

		revision = "sha-2"
		reconcileOnce(r)
		reconcileOnce(r)
		replaced := customObject("Widget", "", "")
		Expect(k8sClient.Get(ctx, widgetKey, &replaced)).To(Succeed())
		Expect(replaced.GetUID()).NotTo(Equal(old.GetUID()), "the next Revision reused the previous hook instead of running it again")
	})

	It("fails the Revision naming a failed hook", func() {
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "pre-sync")
		r := newApplicationReconciler([]unstructured.Unstructured{hook, configMapObject("", "desired")}, nil)
		reconcileOnce(r)
		setWidgetConditions(map[string]any{"type": "Stalled", "status": "True", "message": "migration crashed"})
		reconcileOnce(r)
		failure := latestRevision().Status.Failure
		Expect(failure).NotTo(BeNil())
		Expect(failure.Reason).To(Equal("HookFailed"))
		Expect(failure.Message).To(And(ContainSubstring("Widget/migrate"), ContainSubstring("migration crashed")))
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue())
	})
})
