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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/health"
	"github.com/azrtydxb/kuvryn-sync/internal/resource"
)

var _ = Describe("Application diagnosis", func() {
	const (
		appName = "diagnosed-app"
		secret  = "db-credentials"
	)
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		createRepository(ctx)
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"}})
		deleteObject(ctx, &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-7d9f", Namespace: "payments"}})
		deleteObject(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-7d9f-x2k", Namespace: "payments"}})
		deleteObject(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secret, Namespace: "payments"}})
		deleteApplicationRevisions(ctx, appName)
	})

	desired := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "api"},
		"spec": map[string]any{
			"replicas": int64(1),
			"selector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"app": "api"}},
				"spec": map[string]any{"containers": []any{map[string]any{
					"name": "api", "image": "nginx",
					"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": secret}}},
				}}},
			},
		},
	}}

	reconcileOnce := func(r *ApplicationReconciler) *corev1alpha1.Application {
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		return app
	}
	diagnosedEvents := func(recorder *record.FakeRecorder) []string {
		out := []string{}
		for _, event := range drainEvents(recorder) {
			if strings.Contains(event, "Diagnosed") {
				out = append(out, event)
			}
		}
		return out
	}
	setDeploymentStatus := func(available int32, conditions ...appsv1.DeploymentCondition) {
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "api", Namespace: "payments"}, deployment)).To(Succeed())
		deployment.Status = appsv1.DeploymentStatus{
			ObservedGeneration: deployment.Generation, Replicas: 1, AvailableReplicas: available, ReadyReplicas: available, Conditions: conditions,
		}
		Expect(k8sClient.Status().Update(ctx, deployment)).To(Succeed())
	}
	setPodStatus := func(status corev1.PodStatus) {
		pod := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "api-7d9f-x2k", Namespace: "payments"}, pod)).To(Succeed())
		pod.Status = status
		Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
	}

	It("explains a Deployment degraded by a missing Secret down to the Secret", func() {
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		app.Spec.Strategy.FailurePolicy.MaxAttempts = ptr.To[int32](5)
		// The rollout fails, and the Application becomes Degraded, once the
		// health timeout passes.
		app.Spec.Health.Timeout = &metav1.Duration{Duration: 3 * time.Second}
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		reconciler := newApplicationReconciler([]unstructured.Unstructured{desired}, nil)
		recorder := record.NewFakeRecorder(100)
		reconciler.Recorder = recorder

		By("blaming the missing Secret from the Pod template while the rollout starts")
		updated := reconcileOnce(reconciler)
		Expect(updated.Status.Health.State).To(Equal(corev1alpha1.HealthStateProgressing))
		Expect(updated.Status.Diagnosis).To(HaveLen(1))
		Expect(updated.Status.Diagnosis[0].Reason).To(Equal("MissingSecret"))
		Expect(updated.Status.Diagnosis[0].Chain).To(HaveLen(2))
		events := diagnosedEvents(recorder)
		Expect(events).To(HaveLen(1))
		Expect(events[0]).To(And(HavePrefix("Warning Diagnosed"), ContainSubstring("Secret payments/db-credentials: MissingSecret")))

		By("creating what the Deployment controller and kubelet would: a ReplicaSet and a Pod that cannot start")
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "api", Namespace: "payments"}, deployment)).To(Succeed())
		replicaSet := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "api-7d9f", Namespace: "payments", Labels: map[string]string{"app": "api"}},
			Spec: appsv1.ReplicaSetSpec{
				Replicas: ptr.To[int32](1),
				Selector: deployment.Spec.Selector,
				Template: deployment.Spec.Template,
			},
		}
		Expect(controllerutilSetOwner(deployment, replicaSet)).To(Succeed())
		Expect(k8sClient.Create(ctx, replicaSet)).To(Succeed())
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-7d9f-x2k", Namespace: "payments", Labels: map[string]string{"app": "api"}},
			Spec:       deployment.Spec.Template.Spec,
		}
		Expect(controllerutilSetOwner(replicaSet, pod)).To(Succeed())
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		setPodStatus(corev1.PodStatus{Phase: corev1.PodPending, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "api", Image: "nginx", Ready: false,
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CreateContainerConfigError", Message: `secret "db-credentials" not found`}},
		}}})
		setDeploymentStatus(0)

		By("diagnosing the chain Deployment, ReplicaSet, Pod, Secret while the rollout progresses")
		updated = reconcileOnce(reconciler)
		Expect(updated.Status.Health.State).To(Equal(corev1alpha1.HealthStateProgressing))
		Expect(updated.Status.Diagnosis).To(HaveLen(1))
		Expect(updated.Status.Diagnosis[0].Chain).To(HaveLen(4))
		Expect(diagnosedEvents(recorder)).To(BeEmpty(), "the root cause has not changed")

		By("keeping the diagnosis when the health timeout makes the Application Degraded")
		Eventually(func() corev1alpha1.HealthState {
			updated = reconcileOnce(reconciler)
			return updated.Status.Health.State
		}, 15*time.Second, 500*time.Millisecond).Should(Equal(corev1alpha1.HealthStateDegraded))
		Expect(updated.Status.Diagnosis).To(HaveLen(1))
		cause := updated.Status.Diagnosis[0]
		Expect(cause.Reason).To(Equal("MissingSecret"))
		Expect(cause.Resource).To(Equal(corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "Secret", Namespace: "payments", Name: secret}))
		Expect(cause.Chain).To(Equal([]corev1alpha1.ResourceRef{
			{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "payments", Name: "api"},
			{APIVersion: "apps/v1", Kind: "ReplicaSet", Namespace: "payments", Name: "api-7d9f"},
			{APIVersion: "v1", Kind: "Pod", Namespace: "payments", Name: "api-7d9f-x2k"},
			{APIVersion: "v1", Kind: "Secret", Namespace: "payments", Name: secret},
		}))
		Expect(cause.Message).To(ContainSubstring("CreateContainerConfigError"))
		events = diagnosedEvents(recorder)
		Expect(events).To(HaveLen(1), "becoming Degraded is reported even with the same causes")
		Expect(events[0]).To(ContainSubstring("Secret payments/db-credentials: MissingSecret"))

		By("not repeating the Event while the root causes stay the same, across a retry")
		Eventually(func() int32 {
			reconcileOnce(reconciler)
			return listApplicationRevisions(ctx, appName).Items[0].Status.Attempts
		}, 15*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 2))
		Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
		Expect(updated.Status.Diagnosis).To(HaveLen(1))
		Expect(diagnosedEvents(recorder)).To(BeEmpty())

		By("clearing the diagnosis once the Secret exists and the Deployment is Healthy")
		Expect(k8sClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secret, Namespace: "payments"}})).To(Succeed())
		setPodStatus(corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "api", Image: "nginx", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}}})
		setDeploymentStatus(1, appsv1.DeploymentCondition{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"})
		Eventually(func(g Gomega) {
			updated := reconcileOnce(reconciler)
			g.Expect(updated.Status.Health.State).To(Equal(corev1alpha1.HealthStateHealthy))
			g.Expect(updated.Status.Diagnosis).To(BeEmpty())
		}, 20*time.Second, 500*time.Millisecond).Should(Succeed())
	})
})

// controllerutilSetOwner makes owner the controller of obj, as the built-in
// controllers do.
func controllerutilSetOwner(owner, obj client.Object) error {
	gvk, err := k8sClient.GroupVersionKindFor(owner)
	if err != nil {
		return err
	}
	obj.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: gvk.GroupVersion().String(), Kind: gvk.Kind, Name: owner.GetName(), UID: owner.GetUID(),
		Controller: ptr.To(true), BlockOwnerDeletion: ptr.To(true),
	}})
	return nil
}

func TestDiagnoseEmitsAnEventWhenCausesChangeOrTheApplicationDegrades(t *testing.T) {
	waitingPod := func(reason string) unstructured.Unstructured {
		pod := &corev1.Pod{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments", UID: "pod-uid"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "api:1"}}},
			Status: corev1.PodStatus{Phase: corev1.PodPending, ContainerStatuses: []corev1.ContainerStatus{{
				Name: "api", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: "Back-off"}},
			}}},
		}
		raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
		if err != nil {
			t.Fatal(err)
		}
		return unstructured.Unstructured{Object: raw}
	}
	pulling := waitingPod("ImagePullBackOff")
	podID, err := resource.FromObject(pulling)
	if err != nil {
		t.Fatal(err)
	}
	recorder := record.NewFakeRecorder(10)
	r := &ApplicationReconciler{Recorder: recorder}
	app := newApplication("api", corev1alpha1.RenderTypeYAML)
	tenant := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
	unhealthy := []health.Result{{Resource: podID, State: corev1alpha1.HealthStateProgressing, Reason: "PodPending"}}

	r.diagnose(context.Background(), tenant, app, unhealthy, []unstructured.Unstructured{pulling}, false, corev1alpha1.HealthStateProgressing)
	r.diagnose(context.Background(), tenant, app, unhealthy, []unstructured.Unstructured{pulling}, false, corev1alpha1.HealthStateProgressing)
	if events := drainEvents(recorder); len(events) != 1 || !strings.Contains(events[0], "Pod payments/api: ImagePullBackOff") {
		t.Fatalf("events = %v", events)
	}
	if len(app.Status.Diagnosis) != 1 || app.Status.Diagnosis[0].Reason != "ImagePullBackOff" {
		t.Fatalf("diagnosis = %+v", app.Status.Diagnosis)
	}

	r.diagnose(context.Background(), tenant, app, unhealthy, []unstructured.Unstructured{waitingPod("CrashLoopBackOff")}, false, corev1alpha1.HealthStateProgressing)
	if events := drainEvents(recorder); len(events) != 1 || !strings.Contains(events[0], "CrashLoopBackOff") {
		t.Fatalf("a changed cause must be reported once: %v", events)
	}

	fallback := []health.Result{{Resource: podID, State: corev1alpha1.HealthStateProgressing, Reason: "PodPending"}}
	creating := waitingPod("ContainerCreating")
	r.diagnose(context.Background(), tenant, app, fallback, []unstructured.Unstructured{creating}, false, corev1alpha1.HealthStateProgressing)
	if events := drainEvents(recorder); len(events) != 0 || len(app.Status.Diagnosis) != 1 || app.Status.Diagnosis[0].Reason != "PodPending" {
		t.Fatalf("an ordinary rollout raised events %v with diagnosis %+v", events, app.Status.Diagnosis)
	}
	r.diagnose(context.Background(), tenant, app, fallback, []unstructured.Unstructured{creating}, true, corev1alpha1.HealthStateProgressing)
	if events := drainEvents(recorder); len(events) != 1 {
		t.Fatalf("an Application newly Degraded must report its unchanged fallback cause: %v", events)
	}
	r.diagnose(context.Background(), tenant, app, fallback, []unstructured.Unstructured{creating}, true, corev1alpha1.HealthStateDegraded)
	if events := drainEvents(recorder); len(events) != 0 {
		t.Fatalf("an Application still Degraded with the same causes reported again: %v", events)
	}

	healthy := []health.Result{{Resource: podID, State: corev1alpha1.HealthStateHealthy}}
	r.diagnose(context.Background(), tenant, app, healthy, nil, false, corev1alpha1.HealthStateProgressing)
	if app.Status.Diagnosis != nil || len(drainEvents(recorder)) != 0 {
		t.Fatalf("a Healthy Application keeps diagnosis %+v", app.Status.Diagnosis)
	}
}

func TestRollbackKeepsTheStatusThisReconcileComputed(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	app := newApplication("api", corev1alpha1.RenderTypeYAML)
	app.Spec.Strategy.FailurePolicy.Action = corev1alpha1.FailureActionRollback
	revision := func(name, source string, phase corev1alpha1.RevisionPhase) *corev1alpha1.Revision {
		return &corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{"solder.io/application": "api"}},
			Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: "api"}, Source: corev1alpha1.RevisionSource{Revision: source}},
			Status:     corev1alpha1.RevisionStatus{Phase: phase},
		}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(app, revision("api-old", "old-sha", corev1alpha1.RevisionPhaseHealthy), revision("api-new", "new-sha", corev1alpha1.RevisionPhaseObserving)).
		WithStatusSubresource(&corev1alpha1.Application{}, &corev1alpha1.Revision{}).Build()
	recorder := record.NewFakeRecorder(10)
	r := &ApplicationReconciler{Client: c, Scheme: scheme, Recorder: recorder}
	ctx := context.Background()

	live := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(app), live); err != nil {
		t.Fatal(err)
	}
	current := &corev1alpha1.Revision{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "api-new"}, current); err != nil {
		t.Fatal(err)
	}
	cause := corev1alpha1.DiagnosisCause{
		Resource: corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "Secret", Namespace: "payments", Name: "db"},
		Reason:   "MissingSecret",
		Chain:    []corev1alpha1.ResourceRef{{APIVersion: "v1", Kind: "Secret", Namespace: "payments", Name: "db"}},
	}
	live.Status.Diagnosis = []corev1alpha1.DiagnosisCause{cause}
	live.Status.Resources = corev1alpha1.ResourceHealthSummary{Total: 1, Degraded: 1}
	failure := corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: "One or more resources are degraded", Retryable: true}
	if err := r.failRevisionAndApplication(ctx, live, current, failure); err != nil {
		t.Fatal(err)
	}

	stored := &corev1alpha1.Application{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(app), stored); err != nil {
		t.Fatal(err)
	}
	if stored.GetAnnotations()["solder.io/rollback-revision"] != "old-sha" {
		t.Fatalf("rollback was not requested: %v", stored.GetAnnotations())
	}
	if len(stored.Status.Diagnosis) != 1 || stored.Status.Diagnosis[0].Reason != "MissingSecret" {
		t.Fatalf("diagnosis was lost: %+v", stored.Status.Diagnosis)
	}
	if stored.Status.Resources.Degraded != 1 || stored.Status.Health.State != corev1alpha1.HealthStateDegraded {
		t.Fatalf("health was lost: resources=%+v health=%s", stored.Status.Resources, stored.Status.Health.State)
	}
}

func TestDiagnosisIsClearedByFailuresOutsideHealth(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	stale := []corev1alpha1.DiagnosisCause{{
		Resource: corev1alpha1.ResourceRef{APIVersion: "v1", Kind: "Secret", Namespace: "payments", Name: "db"},
		Reason:   "MissingSecret",
		Chain:    []corev1alpha1.ResourceRef{{APIVersion: "v1", Kind: "Secret", Namespace: "payments", Name: "db"}},
	}}
	ctx := context.Background()
	setup := func(t *testing.T, failure *corev1alpha1.RevisionFailure) (*ApplicationReconciler, *corev1alpha1.Application, *corev1alpha1.Revision) {
		t.Helper()
		app := newApplication("api", corev1alpha1.RenderTypeYAML)
		rev := &corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Name: "api-new", Namespace: "default"},
			Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: "api"}},
			Status:     corev1alpha1.RevisionStatus{Phase: corev1alpha1.RevisionPhaseFailed, Failure: failure},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(app, rev).
			WithStatusSubresource(&corev1alpha1.Application{}, &corev1alpha1.Revision{}).Build()
		live := &corev1alpha1.Application{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(app), live); err != nil {
			t.Fatal(err)
		}
		live.Status.Diagnosis = stale
		current := &corev1alpha1.Revision{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(rev), current); err != nil {
			t.Fatal(err)
		}
		return &ApplicationReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(10)}, live, current
	}
	stored := func(t *testing.T, r *ApplicationReconciler) []corev1alpha1.DiagnosisCause {
		t.Helper()
		app := &corev1alpha1.Application{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: "default", Name: "api"}, app); err != nil {
			t.Fatal(err)
		}
		return app.Status.Diagnosis
	}

	t.Run("source failure", func(t *testing.T) {
		r, app, _ := setup(t, nil)
		r.markApplicationFailure(app, "SourceFailure", "Referenced Repository was not found")
		if app.Status.Diagnosis != nil {
			t.Fatalf("diagnosis kept next to a SourceFailure: %+v", app.Status.Diagnosis)
		}
	})
	t.Run("render failure", func(t *testing.T) {
		r, app, rev := setup(t, nil)
		if err := r.failRevisionAndApplication(ctx, app, rev, corev1alpha1.RevisionFailure{Reason: "RenderFailure", Message: "render failed"}); err != nil {
			t.Fatal(err)
		}
		if got := stored(t, r); got != nil {
			t.Fatalf("diagnosis kept next to a RenderFailure: %+v", got)
		}
	})
	t.Run("health failure", func(t *testing.T) {
		r, app, rev := setup(t, nil)
		if err := r.failRevisionAndApplication(ctx, app, rev, corev1alpha1.RevisionFailure{Reason: "TimeoutFailure", Message: "Health observation timed out"}); err != nil {
			t.Fatal(err)
		}
		if got := stored(t, r); len(got) != 1 {
			t.Fatalf("the diagnosis of a health failure was dropped: %+v", got)
		}
	})
	t.Run("retries blocked after a render failure", func(t *testing.T) {
		r, app, rev := setup(t, &corev1alpha1.RevisionFailure{Reason: "RenderFailure", Message: "render failed"})
		if err := r.reportRetryBlocked(ctx, app, rev, corev1alpha1.RevisionFailure{Reason: "RetryBlocked", Message: "maxAttempts 1 reached"}); err != nil {
			t.Fatal(err)
		}
		if got := stored(t, r); got != nil {
			t.Fatalf("diagnosis kept next to a blocked RenderFailure: %+v", got)
		}
	})
	t.Run("retries blocked after a health failure", func(t *testing.T) {
		r, app, rev := setup(t, &corev1alpha1.RevisionFailure{Reason: "HealthFailure", Message: "One or more resources are degraded"})
		if err := r.reportRetryBlocked(ctx, app, rev, corev1alpha1.RevisionFailure{Reason: "RetryBlocked", Message: "maxAttempts 1 reached"}); err != nil {
			t.Fatal(err)
		}
		if got := stored(t, r); len(got) != 1 {
			t.Fatalf("the diagnosis of a blocked health failure was dropped: %+v", got)
		}
	})
}
