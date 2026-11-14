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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
)

var _ = Describe("Transient API-server failures", func() {
	const appName = "transient-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}
	configKey := types.NamespacedName{Name: "app-config", Namespace: "payments"}

	webhookOutage := apierrors.NewInternalError(errors.New(`failed calling webhook "webhook.cert-manager.io": failed to call webhook: Post "https://cert-manager-webhook.cert-manager.svc:443/validate?timeout=30s": EOF`))
	webhookDenial := apierrors.NewBadRequest(`admission webhook "validate.example.io" denied the request: replicas must be positive`)

	// failure is what the next applies return; nil lets them through.
	var failure error
	var recorder *record.FakeRecorder
	var r *ApplicationReconciler

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		createRepository(ctx)
		attempts := int32(2)
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		app.Spec.Strategy.FailurePolicy.MaxAttempts = &attempts
		Expect(k8sClient.Create(ctx, app)).To(Succeed())

		failure = nil
		recorder = record.NewFakeRecorder(200)
		r = newApplicationReconciler([]unstructured.Unstructured{configMapObject("", "desired")}, nil)
		r.Recorder = recorder
		r.wrapTenant = func(c client.Client) client.Client {
			return applyFailer{Client: c, failure: func() error { return failure }}
		}
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		deleteApplicationRevisions(ctx, appName)
	})

	reconcileOnce := func() reconcile.Result {
		result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		return result
	}
	revision := func() corev1alpha1.Revision {
		revisions := listApplicationRevisions(ctx, appName).Items
		Expect(revisions).To(HaveLen(1))
		return revisions[0]
	}
	ready := func() *metav1.Condition {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		return apimeta.FindStatusCondition(app.Status.Conditions, "Ready")
	}
	// skipBackoff makes a failed Revision's backoff elapse.
	skipBackoff := func() {
		rev := revision()
		rev.Status.CompletedAt = &metav1.Time{Time: time.Now().Add(-time.Hour)}
		Expect(k8sClient.Status().Update(ctx, &rev)).To(Succeed())
	}
	hasEvent := func(events []string, reason string) bool {
		for _, event := range events {
			if strings.Contains(event, " "+reason+" ") {
				return true
			}
		}
		return false
	}

	// Catches a transient outage consuming maxAttempts: on the Dev cluster
	// three webhook EOFs left Applications RetryBlocked for good.
	It("retries a first rollout through an outage without using maxAttempts", func() {
		failure = webhookOutage
		for range 5 {
			result := reconcileOnce()
			rev := revision()
			Expect(rev.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseFailed))
			Expect(rev.Status.Failure).NotTo(BeNil())
			Expect(rev.Status.Failure.Transient).To(BeTrue())
			Expect(rev.Status.Attempts).To(BeNumerically("<=", 1), "a transient failure used an attempt")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(ready().Reason).NotTo(Equal("RetryBlocked"))
			skipBackoff()
		}
		Expect(revision().Status.TransientFailures).To(Equal(int32(5)))
		Expect(hasEvent(drainEvents(recorder), "TransientFailure")).To(BeTrue())

		failure = nil
		reconcileOnce()
		rev := revision()
		Expect(rev.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(rev.Status.TransientFailures).To(BeZero())
		Expect(k8sClient.Get(ctx, configKey, &corev1.ConfigMap{})).To(Succeed())
		Expect(ready().Status).To(Equal(metav1.ConditionTrue))
	})

	// Catches a self-heal repair that hit a webhook outage marking its
	// Healthy Revision Failed and, after maxAttempts, RetryBlocked.
	It("leaves a Healthy Revision Healthy when a self-heal hits an outage", func() {
		// A hand edit takes over a field; adopting it keeps the repair from
		// failing as a conflict, which is not what this test is about.
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		app.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyAdopt
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
		reconcileOnce()
		Expect(revision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		attempts := revision().Status.Attempts

		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, configKey, live)).To(Succeed())
		live.Data["key"] = "edited-by-hand"
		Expect(k8sClient.Update(ctx, live)).To(Succeed())

		failure = webhookOutage
		for range 4 {
			result := reconcileOnce()
			rev := revision()
			Expect(rev.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy), "failure: %+v", rev.Status.Failure)
			Expect(rev.Status.Failure).To(BeNil())
			Expect(rev.Status.Attempts).To(Equal(attempts))
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(ready().Reason).NotTo(Equal("RetryBlocked"))
			Expect(ready().Status).To(Equal(metav1.ConditionTrue))
		}
		Expect(revision().Status.TransientFailures).To(Equal(int32(4)))
		Expect(revision().Status.LastTransientFailureAt).NotTo(BeNil())
		Expect(hasEvent(drainEvents(recorder), "TransientFailure")).To(BeTrue())

		failure = nil
		reconcileOnce()
		Expect(k8sClient.Get(ctx, configKey, live)).To(Succeed())
		Expect(live.Data).To(HaveKeyWithValue("key", "desired"), "the repair never happened after the outage")
		rev := revision()
		Expect(rev.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(rev.Status.TransientFailures).To(BeZero())
	})

	// Catches a classifier that retries denials forever: a webhook that
	// rejected the object is a verdict, not an outage.
	It("still blocks after maxAttempts when a webhook denies the object", func() {
		failure = webhookDenial
		reconcileOnce()
		rev := revision()
		Expect(rev.Status.Failure.Transient).To(BeFalse())
		Expect(rev.Status.Attempts).To(Equal(int32(1)))
		skipBackoff()
		reconcileOnce()
		Expect(revision().Status.Attempts).To(Equal(int32(2)))
		skipBackoff()
		reconcileOnce()
		Expect(ready().Reason).To(Equal("RetryBlocked"))
		Expect(revision().Status.Attempts).To(Equal(int32(2)))
		Expect(hasEvent(drainEvents(recorder), "TransientFailure")).To(BeFalse())
	})

	// Catches a blocked Revision that no request can unblock.
	It("gives a blocked Revision a fresh maxAttempts on a retry request", func() {
		failure = webhookDenial
		for range 3 {
			reconcileOnce()
			skipBackoff()
		}
		Expect(ready().Reason).To(Equal("RetryBlocked"))
		drainEvents(recorder)

		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		app.Annotations = map[string]string{corev1alpha1.RetryRequestedAnnotation: "2026-11-14T10:00:00Z"}
		Expect(k8sClient.Update(ctx, app)).To(Succeed())

		// The retry starts at once and uses one attempt of the fresh budget.
		reconcileOnce()
		rev := revision()
		Expect(rev.Status.Attempts).To(Equal(int32(1)))
		Expect(rev.Status.HandledRetryRequest).To(Equal("2026-11-14T10:00:00Z"))
		Expect(hasEvent(drainEvents(recorder), "RetryRequested")).To(BeTrue())

		By("honoring one request only once")
		skipBackoff()
		reconcileOnce()
		skipBackoff()
		reconcileOnce()
		Expect(revision().Status.Attempts).To(Equal(int32(2)))
		Expect(ready().Reason).To(Equal("RetryBlocked"))

		By("recovering once the cause is fixed")
		app = &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		app.Annotations[corev1alpha1.RetryRequestedAnnotation] = "2026-11-14T10:05:00Z"
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
		failure = nil
		reconcileOnce()
		Expect(revision().Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		Expect(ready().Status).To(Equal(metav1.ConditionTrue))
	})

	// Catches an old request annotation resetting a new Revision's first
	// failures: it is recorded when the Revision first sees it.
	It("does not let a request made before a Revision existed reset it", func() {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		app.Annotations = map[string]string{corev1alpha1.RetryRequestedAnnotation: "2026-11-14T09:00:00Z"}
		Expect(k8sClient.Update(ctx, app)).To(Succeed())

		failure = webhookDenial
		reconcileOnce()
		skipBackoff()
		reconcileOnce()
		skipBackoff()
		reconcileOnce()
		Expect(ready().Reason).To(Equal("RetryBlocked"))
		Expect(revision().Status.Attempts).To(Equal(int32(2)))
	})

})

// applyFailer fails server-side applies with what failure returns, as an
// admission webhook outage would.
type applyFailer struct {
	client.Client
	failure func() error
}

func (a applyFailer) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	if err := a.failure(); err != nil {
		return err
	}
	return a.Client.Apply(ctx, obj, opts...)
}
