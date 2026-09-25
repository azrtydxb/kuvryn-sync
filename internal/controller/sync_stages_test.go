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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/source"
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

	updateApplication := func(mutate func(*corev1alpha1.Application)) {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		mutate(app)
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
	}
	application := func() *corev1alpha1.Application {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		return app
	}
	markDeploymentReady := func() {
		live := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "api", Namespace: "payments"}, live)).To(Succeed())
		live.Status.ObservedGeneration, live.Status.Replicas, live.Status.AvailableReplicas, live.Status.ReadyReplicas, live.Status.UpdatedReplicas = live.Generation, 1, 1, 1, 1
		Expect(k8sClient.Status().Update(ctx, live)).To(Succeed())
	}
	widgetExists := func() bool {
		widget := customObject("Widget", "", "")
		err := k8sClient.Get(ctx, widgetKey, &widget)
		Expect(client.IgnoreNotFound(err)).To(Succeed())
		return err == nil
	}
	deleteWidget := func() {
		widget := customObject("Widget", "migrate", "")
		widget.SetNamespace("payments")
		Expect(k8sClient.Delete(ctx, &widget)).To(Succeed())
		Eventually(widgetExists).Should(BeFalse())
	}
	countEvents := func(events []string, reason string) int {
		n := 0
		for _, event := range events {
			if strings.Contains(event, " "+reason+" ") {
				n++
			}
		}
		return n
	}

	DescribeTable("reports drift without reverting it when self-heal is off", func(policy corev1alpha1.ConflictPolicy) {
		// Nothing is applied while drifted, so even a hand edit that took
		// over a field is reported rather than failed as a conflict.
		updateApplication(func(app *corev1alpha1.Application) {
			app.Spec.Sync.SelfHeal = false
			app.Spec.Sync.ConflictPolicy = policy
		})
		r := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		reconcileOnce(r)
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))

		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, configKey, live)).To(Succeed())
		live.Data["key"] = "edited-by-hand"
		Expect(k8sClient.Update(ctx, live)).To(Succeed())
		for range 3 {
			reconcileOnce(r)
			Expect(k8sClient.Get(ctx, configKey, live)).To(Succeed())
			Expect(live.Data).To(HaveKeyWithValue("key", "edited-by-hand"), "self-heal is off, yet the edit was reverted")
			Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateDrifted))
		}
		// Drift keeps the finished rollout's phase and last observed health.
		Expect(application().Status.Health.State).To(Equal(corev1alpha1.HealthStateHealthy))
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))

		// Undoing the edit settles the Application without a new rollout.
		recorder := record.NewFakeRecorder(50)
		r.Recorder = recorder
		live.Data["key"] = "desired"
		Expect(k8sClient.Update(ctx, live)).To(Succeed())
		reconcileOnce(r)
		Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))
		Expect(countEvents(drainEvents(recorder), "DeploymentStarted")).To(BeZero())
	},
		Entry("with the fail conflict policy", corev1alpha1.ConflictPolicyFail),
		Entry("with the adopt conflict policy", corev1alpha1.ConflictPolicyAdopt),
	)

	It("resumes a rollout a dependency interrupted instead of calling it drift", func() {
		operatorKey := types.NamespacedName{Name: "operator", Namespace: "default"}
		operator := newApplication(operatorKey.Name, corev1alpha1.RenderTypeYAML)
		Expect(k8sClient.Create(ctx, operator)).To(Succeed())
		DeferCleanup(func() {
			deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: operatorKey.Name, Namespace: operatorKey.Namespace}})
		})
		setOperatorHealth := func(state corev1alpha1.HealthState) {
			Expect(k8sClient.Get(ctx, operatorKey, operator)).To(Succeed())
			operator.Status.ObservedGeneration = operator.Generation
			operator.Status.Health.State = state
			operator.Status.DesiredRevision, operator.Status.DeployedRevision = "sha-1", "sha-1"
			Expect(k8sClient.Status().Update(ctx, operator)).To(Succeed())
		}
		updateApplication(func(app *corev1alpha1.Application) {
			app.Spec.Sync.SelfHeal = false
			app.Spec.DependsOn = []corev1alpha1.LocalObjectReference{{Name: operatorKey.Name}}
		})
		r := newApplicationReconciler([]unstructured.Unstructured{deployment("0"), annotate(configMapObject("", "desired"), "solder.io/sync-wave", "1")}, nil)
		setOperatorHealth(corev1alpha1.HealthStateHealthy)
		reconcileOnce(r)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "api", Namespace: "payments"}, &appsv1.Deployment{})).To(Succeed())

		setOperatorHealth(corev1alpha1.HealthStateProgressing)
		markDeploymentReady()
		reconcileOnce(r)
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue(), "wave 1 applied while a dependency was not Healthy")

		setOperatorHealth(corev1alpha1.HealthStateHealthy)
		reconcileOnce(r)
		Expect(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{})).To(Succeed(), "the interrupted rollout was treated as drift and never finished")
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))
	})

	It("retries a failed rollout instead of calling it drift", func() {
		attempts := int32(3)
		updateApplication(func(app *corev1alpha1.Application) {
			app.Spec.Sync.SelfHeal = false
			app.Spec.Strategy.FailurePolicy.MaxAttempts = &attempts
		})
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "pre-sync")
		r := newApplicationReconciler([]unstructured.Unstructured{hook, configMapObject("", "desired")}, nil)
		reconcileOnce(r)
		Expect(application().Status.DeployedRevision).To(Equal("resolved-sha"))
		setWidgetConditions(map[string]any{"type": "Stalled", "status": "True", "message": "database locked"})
		reconcileOnce(r)
		failed := latestRevision()
		Expect(failed.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseFailed))

		// Skip the retry backoff, then let the hook recover.
		failed.Status.CompletedAt = &metav1.Time{Time: time.Now().Add(-time.Hour)}
		Expect(k8sClient.Status().Update(ctx, &failed)).To(Succeed())
		setWidgetConditions(map[string]any{"type": "Ready", "status": "True"})
		reconcileOnce(r)
		Expect(application().Status.Sync.State).NotTo(Equal(corev1alpha1.SyncStateDrifted))
		Expect(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{})).To(Succeed(), "the retry was treated as drift and never applied")
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
	})

	It("finishes a manual multi-group rollout on a single approval", func() {
		updateApplication(func(app *corev1alpha1.Application) { app.Spec.Sync.Automatic = false })
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "pre-sync")
		r := newApplicationReconciler([]unstructured.Unstructured{hook, deployment("0"), annotate(configMapObject("", "desired"), "solder.io/sync-wave", "1")}, nil)
		reconcileOnce(r)
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))
		approve(ctx, key, latestRevision(), "alice@example.com")

		reconcileOnce(r)
		Expect(widgetExists()).To(BeTrue())
		setWidgetConditions(map[string]any{"type": "Ready", "status": "True"})
		reconcileOnce(r)
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseObserving), "the rollout stopped for approval after the pre-sync hook")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "api", Namespace: "payments"}, &appsv1.Deployment{})).To(Succeed())

		markDeploymentReady()
		reconcileOnce(r)
		Expect(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{})).To(Succeed())
		done := latestRevision()
		Expect(done.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(done.Status.Approval).NotTo(BeNil())
		Expect(done.Status.Approval.ApprovedBy).To(Equal("alice@example.com"))
	})

	It("asks for a fresh approval when desired state changes during a manual rollout", func() {
		updateApplication(func(app *corev1alpha1.Application) { app.Spec.Sync.Automatic = false })
		capture := &capturingRenderer{objects: []unstructured.Unstructured{deployment("0"), annotate(configMapObject("", "reviewed"), "solder.io/sync-wave", "1")}}
		r := newApplicationReconciler(nil, capture)
		reconcileOnce(r)
		approve(ctx, key, latestRevision(), "alice@example.com")
		reconcileOnce(r)
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseObserving))

		capture.objects = []unstructured.Unstructured{deployment("0"), annotate(configMapObject("", "changed-after-review"), "solder.io/sync-wave", "1")}
		markDeploymentReady()
		reconcileOnce(r)
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue(), "changed desired state was applied on an old approval")
	})

	// Catches a PruneSkipped Event on every reconcile of a rollout, which
	// walks its groups, and prunes, again each time it checks a hook.
	It("warns about skipped prunes once while a post-sync hook runs", func() {
		Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kept-config", Namespace: "payments",
				Labels:      map[string]string{"solder.io/application": appName},
				Annotations: map[string]string{"solder.io/prune": "disabled"},
			},
		})).To(Succeed())
		DeferCleanup(func() {
			deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kept-config", Namespace: "payments"}})
		})
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "post-sync")
		r := newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired"), hook}, nil)
		recorder := record.NewFakeRecorder(100)
		r.Recorder = recorder
		for range 4 {
			reconcileOnce(r)
			Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseObserving), "the post-sync hook should still be running")
		}
		Expect(widgetExists()).To(BeTrue())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "kept-config", Namespace: "payments"}, &corev1.ConfigMap{})).To(Succeed())
		Expect(countEvents(drainEvents(recorder), "PruneSkipped")).To(Equal(1))

		setWidgetConditions(map[string]any{"type": "Ready", "status": "True"})
		reconcileOnce(r)
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(countEvents(drainEvents(recorder), "PruneSkipped")).To(BeZero())
	})

	It("never runs a succeeded hook again, even after it is cleaned up", func() {
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "pre-sync")
		r := newApplicationReconciler([]unstructured.Unstructured{hook, deployment("0")}, nil)
		reconcileOnce(r)
		setWidgetConditions(map[string]any{"type": "Ready", "status": "True"})
		reconcileOnce(r)
		Expect(latestRevision().Status.Hooks).To(ConsistOf(HaveField("State", corev1alpha1.HealthStateHealthy)))

		// Like a Job removed by ttlSecondsAfterFinished.
		deleteWidget()
		reconcileOnce(r)
		Expect(widgetExists()).To(BeFalse(), "the succeeded hook ran again mid-rollout")
		markDeploymentReady()
		reconcileOnce(r)
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))

		reconcileOnce(r)
		Expect(widgetExists()).To(BeFalse(), "self-heal ran the succeeded hook again")
		app := application()
		Expect(app.Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))
		Expect(latestRevision().Status.Plan.Summary.Create).To(BeZero(), "a cleaned-up hook was planned as drift")
	})

	It("fails a hook that disappears before it succeeded", func() {
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "pre-sync")
		r := newApplicationReconciler([]unstructured.Unstructured{hook, configMapObject("", "desired")}, nil)
		reconcileOnce(r)
		deleteWidget()
		reconcileOnce(r)
		failure := latestRevision().Status.Failure
		Expect(failure).NotTo(BeNil())
		Expect(failure.Reason).To(Equal("HookFailed"))
		Expect(failure.Message).To(And(ContainSubstring("Widget/migrate"), ContainSubstring("deleted")))
		Expect(widgetExists()).To(BeFalse())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue())
	})

	It("re-creates a failed hook an operator deleted when the rollout is retried", func() {
		attempts := int32(3)
		updateApplication(func(app *corev1alpha1.Application) { app.Spec.Strategy.FailurePolicy.MaxAttempts = &attempts })
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "pre-sync")
		r := newApplicationReconciler([]unstructured.Unstructured{hook, configMapObject("", "desired")}, nil)
		reconcileOnce(r)
		setWidgetConditions(map[string]any{"type": "Stalled", "status": "True", "message": "database locked"})
		reconcileOnce(r)
		failed := latestRevision()
		Expect(failed.Status.Failure).NotTo(BeNil())
		Expect(failed.Status.Failure.Reason).To(Equal("HookFailed"))

		deleteWidget()
		failed.Status.CompletedAt = &metav1.Time{Time: time.Now().Add(-time.Hour)}
		Expect(k8sClient.Status().Update(ctx, &failed)).To(Succeed())
		reconcileOnce(r)
		Expect(widgetExists()).To(BeTrue(), "the retry did not run the failed hook again")
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseObserving))
	})

	It("announces a deployment once while observing it", func() {
		recorder := record.NewFakeRecorder(100)
		r := newApplicationReconciler([]unstructured.Unstructured{deployment("0")}, nil)
		r.Recorder = recorder
		for range 3 {
			reconcileOnce(r)
		}
		Expect(latestRevision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseObserving))
		Expect(countEvents(drainEvents(recorder), "DeploymentStarted")).To(Equal(1))
	})

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
		// Without self-heal, a rollout still in progress must not be mistaken
		// for drift of a finished one.
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		app.Spec.Sync.SelfHeal = false
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
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

	It("refuses an unknown solder.io/hook value instead of applying it", func() {
		hook := annotate(customObject("Widget", "migrate", "v1"), "solder.io/hook", "pre-install")
		r := newApplicationReconciler([]unstructured.Unstructured{hook, configMapObject("", "desired")}, nil)
		reconcileOnce(r)
		failure := latestRevision().Status.Failure
		Expect(failure).NotTo(BeNil())
		Expect(failure.Reason).To(Equal("ValidationFailure"))
		Expect(failure.Message).To(And(ContainSubstring("Widget migrate"), ContainSubstring("pre-install")))
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{}))).To(BeTrue())
		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, widgetKey, &unstructured.Unstructured{Object: map[string]any{"apiVersion": "example.com/v1", "kind": "Widget"}}))).To(BeTrue())
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
