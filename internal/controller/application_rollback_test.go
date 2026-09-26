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
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/renderer"
	"github.com/azrtydxb/kuvryn-sync/internal/revisionid"
	"github.com/azrtydxb/kuvryn-sync/internal/source"
)

// namedWorkspaceResolver resolves the branch "main" to *revision and any
// other ref, such as a rollback target, to itself, in a workspace named after
// the commit, so a renderer can tell commits apart.
// A ref named in unreachable fails to resolve, like a fetch that fails.
type namedWorkspaceResolver struct {
	revision    *string
	unreachable map[string]bool
}

func (r namedWorkspaceResolver) Resolve(_ context.Context, repository source.GitRepository) (source.ResolvedSource, error) {
	if r.unreachable[repository.Revision] {
		return source.ResolvedSource{}, errors.New("fetch failed")
	}
	resolved := repository.Revision
	if resolved == "main" {
		resolved = *r.revision
	}
	return source.ResolvedSource{Revision: resolved, CacheDir: "/tmp/kuvryn-sync-workspace-" + resolved}, nil
}

type renderFunc func(renderer.Input) ([]unstructured.Unstructured, error)

func (f renderFunc) Render(_ context.Context, input renderer.Input) ([]unstructured.Unstructured, error) {
	return f(input)
}

// commitRenderer renders a ConfigMap holding the commit, plus extra objects
// for some commits, and fails to render the failing ones.
func commitRenderer(extra map[string][]unstructured.Unstructured, failing ...string) RendererFactory {
	return func(corev1alpha1.RenderType) (renderer.Renderer, error) {
		return renderFunc(func(input renderer.Input) ([]unstructured.Unstructured, error) {
			commit := strings.TrimPrefix(input.Workspace, "/tmp/kuvryn-sync-workspace-")
			if slices.Contains(failing, commit) {
				return nil, errors.New("render failed")
			}
			return append([]unstructured.Unstructured{configMapObject("", commit)}, extra[commit]...), nil
		}), nil
	}
}

var _ = Describe("Rollbacks", func() {
	const appName = "rollback-app"
	ctx := context.Background()
	key := types.NamespacedName{Name: appName, Namespace: "default"}
	widgetKey := types.NamespacedName{Name: "health", Namespace: "payments"}
	var source string
	var r *ApplicationReconciler

	BeforeEach(func() {
		ensureNamespace(ctx, "payments")
		ensureCustomKind(ctx, "Widget", "widgets")
		Expect(k8sClient.Create(ctx, &corev1alpha1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{Name: "rollback-widgets-running"},
			Spec: corev1alpha1.HealthCheckSpec{Group: "example.com", Kind: "Widget", Rules: []corev1alpha1.HealthRule{{
				Expression: "!has(object.status)", State: corev1alpha1.HealthStateProgressing, Message: "not started",
			}}},
		})).To(Succeed())
		createRepository(ctx)
		app := newApplication(appName, corev1alpha1.RenderTypeYAML)
		app.Spec.Sync.Automatic = true
		Expect(k8sClient.Create(ctx, app)).To(Succeed())
		source = "a-sha"
		r = newApplicationReconciler(nil, nil)
		r.SourceResolver = namedWorkspaceResolver{revision: &source}
		r.Renderers = commitRenderer(nil)
	})

	AfterEach(func() {
		deleteObject(ctx, &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"}})
		deleteObject(ctx, &corev1alpha1.HealthCheck{ObjectMeta: metav1.ObjectMeta{Name: "rollback-widgets-running"}})
		deleteObject(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}})
		for _, name := range []string{"health", "migrate"} {
			widget := customObject("Widget", name, "")
			widget.SetNamespace("payments")
			_ = k8sClient.Delete(ctx, &widget)
		}
		deleteApplicationRevisions(ctx, appName)
	})

	reconcileOnce := func() {
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
	}
	application := func() *corev1alpha1.Application {
		app := &corev1alpha1.Application{}
		Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
		return app
	}
	ready := func() *metav1.Condition {
		condition := apimeta.FindStatusCondition(application().Status.Conditions, ReadyCondition)
		Expect(condition).NotTo(BeNil())
		return condition
	}
	// deployed is the commit whose ConfigMap is live.
	deployed := func() string {
		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, live)).To(Succeed())
		return live.Data["key"]
	}
	revisionsOf := func(commit string) []corev1alpha1.Revision {
		out := []corev1alpha1.Revision{}
		for _, rev := range listApplicationRevisions(ctx, appName).Items {
			if rev.Spec.Source.Revision == commit {
				out = append(out, rev)
			}
		}
		return out
	}
	expectHeld := func(commit, reason string) {
		revisions := revisionsOf(commit)
		Expect(revisions).NotTo(BeEmpty())
		for _, rev := range revisions {
			Expect(rev.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseFailed), rev.Name)
			held := heldBy(&rev)
			Expect(held).NotTo(BeNil(), rev.Name)
			Expect(held.Reason).To(Equal(reason))
		}
	}
	// requestAsWebhook requests a manual rollback to target as `ksync
	// rollback` does, with the record the admission webhook adds; this suite
	// runs without it.
	requestAsWebhook := func(target corev1alpha1.Revision, from, by string, at time.Time) {
		app := application()
		setRollbackRequest(app, rollbackRequest{target: target.Spec.Source.Revision, from: from, kind: corev1alpha1.RollbackKindManual})
		app.Annotations[corev1alpha1.RollbackTargetRevisionAnnotation] = target.Name
		app.Annotations[corev1alpha1.RollbackRequestedByAnnotation] = by
		app.Annotations[corev1alpha1.RollbackRequestedAtAnnotation] = at.Format(time.RFC3339)
		app.Annotations[corev1alpha1.RollbackTargetHashAnnotation] = revisionid.RollbackBinding(&target, app)
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
	}
	requestRollback := func(target, from string) {
		app := application()
		setRollbackRequest(app, rollbackRequest{target: target, from: from, kind: corev1alpha1.RollbackKindManual})
		Expect(k8sClient.Update(ctx, app)).To(Succeed())
	}
	setWidget := func(conditions ...map[string]any) {
		widget := customObject("Widget", "", "")
		Expect(k8sClient.Get(ctx, widgetKey, &widget)).To(Succeed())
		list := make([]any, 0, len(conditions))
		for _, c := range conditions {
			list = append(list, c)
		}
		Expect(unstructured.SetNestedSlice(widget.Object, list, "status", "conditions")).To(Succeed())
		Expect(k8sClient.Update(ctx, &widget)).To(Succeed())
	}
	expectHolding := func(on, held string) {
		for range 3 {
			reconcileOnce()
			Expect(deployed()).To(Equal(on), "the held revision was deployed again")
			app := application()
			Expect(app.GetAnnotations()).NotTo(HaveKey(corev1alpha1.RollbackRevisionAnnotation))
			Expect(app.Status.DeployedRevision).To(Equal(on))
			Expect(app.Status.DesiredRevision).To(Equal(held))
			Expect(app.Status.Sync.State).To(Equal(corev1alpha1.SyncStateOutOfSync))
			Expect(app.Status.Health.State).To(Equal(corev1alpha1.HealthStateHealthy))
			condition := ready()
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal("RolledBack"))
			Expect(condition.ObservedGeneration).To(Equal(app.Generation))
		}
	}
	expectNewCommitDeploys := func() {
		source = "c-sha"
		reconcileOnce()
		Expect(deployed()).To(Equal("c-sha"))
		Expect(ready().Status).To(Equal(metav1.ConditionTrue))
		Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))
	}

	// Catches an automatic rollback being undone: the failed Revision it
	// replaced was left RollingBack, so the next reconcile retried it.
	It("holds an automatic rollback until a new commit arrives", func() {
		updateApp := application()
		updateApp.Spec.Strategy.FailurePolicy.Action = corev1alpha1.FailureActionRollback
		Expect(k8sClient.Update(ctx, updateApp)).To(Succeed())
		r.Renderers = commitRenderer(nil, "b-sha")
		reconcileOnce()
		// A failure policy rolls back to an older Revision, by creation time.
		time.Sleep(1100 * time.Millisecond)
		source = "b-sha"
		reconcileOnce()
		Expect(application().GetAnnotations()).To(HaveKeyWithValue(corev1alpha1.RollbackFromAnnotation, "b-sha"))
		Expect(application().GetAnnotations()).To(HaveKeyWithValue(corev1alpha1.RollbackKindAnnotation, corev1alpha1.RollbackKindAutomatic))

		reconcileOnce()
		expectHolding("a-sha", "b-sha")
		expectHeld("b-sha", "RollbackCompleted")
		Expect(ready().Message).To(ContainSubstring("after a failure"))
		expectNewCommitDeploys()
	})

	// Catches a manual rollback being undone, including Revisions created in
	// the same second, which creation-time ordering could not tell apart.
	It("holds a manual rollback until a new commit arrives", func() {
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"))

		requestRollback("a-sha", "b-sha")
		reconcileOnce()
		Expect(deployed()).To(Equal("a-sha"))
		expectHolding("a-sha", "b-sha")
		expectHeld("b-sha", "ManualRollback")
		Expect(ready().Message).To(ContainSubstring("rolled back manually"))
		for _, rev := range revisionsOf("a-sha") {
			Expect(heldBy(&rev)).To(BeNil(), "the rollback target was held")
		}
		expectNewCommitDeploys()
	})

	// Catches a manual rollback on a manual-approval Application stopping at
	// AwaitingApproval: the target was re-planned against live state, its
	// new plan digest matched no approval, and nothing deployed until a
	// separate `ksync sync`. The rollback request is the approval.
	It("deploys a manual rollback on a manual-approval Application without a separate approval", func() {
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"))
		for _, commit := range []string{"a-sha", "b-sha"} {
			Expect(revisionsOf(commit)).To(HaveLen(1))
			Expect(revisionsOf(commit)[0].Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy), commit)
		}
		manual := application()
		manual.Spec.Sync.Automatic = false
		Expect(k8sClient.Update(ctx, manual)).To(Succeed())
		reconcileOnce()
		Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))

		requestedAt := time.Now().UTC().Truncate(time.Second)
		requestAsWebhook(revisionsOf("a-sha")[0], "b-sha", "alice@example.com", requestedAt)
		reconcileOnce()

		Expect(deployed()).To(Equal("a-sha"), "the rollback target waited for a separate approval")
		target := revisionsOf("a-sha")[0]
		Expect(target.Status.Phase).To(Equal(corev1alpha1.RevisionPhaseRolledBack))
		Expect(target.Status.Approval).NotTo(BeNil())
		Expect(target.Status.Approval.ApprovedBy).To(Equal("alice@example.com"))
		Expect(target.Status.Approval.ApprovedAt.Time.Equal(requestedAt)).To(BeTrue(), "approvedAt %s", target.Status.Approval.ApprovedAt)
		Expect(target.Status.Approval.PlanDigest).To(Equal(target.Status.Plan.Digest), "the approval is not for the plan that was applied")
		Expect(target.Status.StartedAt).NotTo(BeNil())
		Expect(target.Status.StartedAt.Time).NotTo(BeTemporally("<", requestedAt), "the redeployed target kept its first rollout's start")
		expectHolding("a-sha", "b-sha")
		expectHeld("b-sha", "ManualRollback")
	})

	// Catches a manual rollback approving configuration nobody chose: the
	// controller built the rollback's Revision from the Application's spec
	// as it is now, so a render, path or values change merged after the
	// requester picked a known-good Revision was deployed under their name.
	for _, change := range []struct {
		name  string
		apply func()
	}{
		{"spec.source.render changes", func() {
			changed := application()
			changed.Spec.Source.Render.Type = corev1alpha1.RenderTypeKustomize
			Expect(k8sClient.Update(ctx, changed)).To(Succeed())
		}},
		{"a Helm value changes the target's desired state", func() {
			// A valuesFrom Secret changes what the same Revision renders.
			r.Renderers = func(corev1alpha1.RenderType) (renderer.Renderer, error) {
				return renderFunc(func(input renderer.Input) ([]unstructured.Unstructured, error) {
					commit := strings.TrimPrefix(input.Workspace, "/tmp/kuvryn-sync-workspace-")
					if commit == "a-sha" {
						commit = "a-sha with new values"
					}
					return []unstructured.Unstructured{configMapObject("", commit)}, nil
				}), nil
			}
		}},
	} {
		It("does not approve a manual rollback when "+change.name+" while it is pending", func() {
			recorder := record.NewFakeRecorder(200)
			r.Recorder = recorder
			reconcileOnce()
			source = "b-sha"
			reconcileOnce()
			manual := application()
			manual.Spec.Sync.Automatic = false
			Expect(k8sClient.Update(ctx, manual)).To(Succeed())
			reconcileOnce()

			chosen := revisionsOf("a-sha")[0]
			requestAsWebhook(chosen, "b-sha", "alice@example.com", time.Now().UTC())
			change.apply()
			reconcileOnce()

			Expect(deployed()).To(Equal("b-sha"), "a rollback target nobody chose was deployed")
			Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateAwaitingApproval))
			for _, rev := range revisionsOf("a-sha") {
				Expect(rev.Status.Phase).NotTo(Equal(corev1alpha1.RevisionPhaseRolledBack), rev.Name)
				if rev.Status.Phase == corev1alpha1.RevisionPhaseAwaitingApproval && rev.Status.Approval != nil {
					Expect(rev.Status.Approval.ApprovedBy).NotTo(Equal("alice@example.com"), "the requester approved a target that changed")
				}
			}
			events := drainEvents(recorder)
			Expect(events).To(ContainElement(HavePrefix("Warning ApprovalStale")))
			Expect(events).To(ContainElement(And(HavePrefix("Warning RollbackTargetChanged"), ContainSubstring("ksync sync"))))
			Expect(events).NotTo(ContainElement(HavePrefix("Normal RollbackApproved")))
		})
	}

	// Catches a rollback approving values nobody deployed: the webhook
	// recorded the target's latest render, and a drift check had re-rendered
	// it with values someone set after it was deployed, so the rollback
	// deployed those values under the requester's name.
	It("does not approve a rollback to a Revision re-rendered with other values since it deployed", func() {
		recorder := record.NewFakeRecorder(200)
		r.Recorder = recorder
		noSelfHeal := application()
		noSelfHeal.Spec.Sync.SelfHeal = false
		Expect(k8sClient.Update(ctx, noSelfHeal)).To(Succeed())
		reconcileOnce()
		reconcileOnce()
		Expect(deployed()).To(Equal("a-sha"))
		Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))
		Expect(revisionsOf("a-sha")[0].Status.Phase).To(Equal(corev1alpha1.RevisionPhaseHealthy))
		deployedHash := revisionsOf("a-sha")[0].Spec.DesiredStateHash

		// Someone who may edit a valuesFrom Secret but not approve changes
		// what a-sha renders; without selfHeal it only shows as drift.
		evil := func(corev1alpha1.RenderType) (renderer.Renderer, error) {
			return renderFunc(func(input renderer.Input) ([]unstructured.Unstructured, error) {
				commit := strings.TrimPrefix(input.Workspace, "/tmp/kuvryn-sync-workspace-")
				if commit == "a-sha" {
					commit = "a-sha with evil values"
				}
				return []unstructured.Unstructured{configMapObject("", commit)}, nil
			}), nil
		}
		r.Renderers = evil
		reconcileOnce()
		Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateDrifted))
		Expect(deployed()).To(Equal("a-sha"))
		Expect(revisionsOf("a-sha")[0].Spec.DesiredStateHash).NotTo(Equal(deployedHash), "the drift check did not re-render the Revision")

		source = "b-sha"
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"))
		manual := application()
		manual.Spec.Sync.Automatic = false
		Expect(k8sClient.Update(ctx, manual)).To(Succeed())
		reconcileOnce()

		requestAsWebhook(revisionsOf("a-sha")[0], "b-sha", "alice@example.com", time.Now().UTC())
		reconcileOnce()

		Expect(deployed()).To(Equal("b-sha"), "values nobody deployed were rolled out under the requester's name")
		Expect(drainEvents(recorder)).NotTo(ContainElement(HavePrefix("Normal RollbackApproved")))
	})

	// Catches a failure policy's rollback on a manual-approval Application
	// deploying without approval: only a person's request approves.
	It("keeps an automatic rollback on a manual-approval Application waiting for approval", func() {
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		manual := application()
		manual.Spec.Sync.Automatic = false
		Expect(k8sClient.Update(ctx, manual)).To(Succeed())

		requested := application()
		setRollbackRequest(requested, rollbackRequest{target: "a-sha", from: "b-sha", kind: corev1alpha1.RollbackKindAutomatic})
		requested.Annotations[corev1alpha1.RollbackRequestedByAnnotation] = "system:serviceaccount:kuvryn-sync-system:kuvryn-sync-controller-manager"
		requested.Annotations[corev1alpha1.RollbackRequestedAtAnnotation] = time.Now().UTC().Format(time.RFC3339)
		Expect(k8sClient.Update(ctx, requested)).To(Succeed())
		reconcileOnce()

		Expect(deployed()).To(Equal("b-sha"))
		Expect(revisionsOf("a-sha")[0].Status.Phase).To(Equal(corev1alpha1.RevisionPhaseAwaitingApproval))
		Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateAwaitingApproval))
	})

	// Catches a rollback away from a commit that failed, whose Failed
	// Revision was not held, so the Application ended RetryBlocked.
	It("holds a failed commit a manual rollback replaces", func() {
		r.Renderers = commitRenderer(nil, "b-sha")
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		Expect(revisionsOf("b-sha")[0].Status.Phase).To(Equal(corev1alpha1.RevisionPhaseFailed))

		requestRollback("a-sha", "b-sha")
		reconcileOnce()
		expectHolding("a-sha", "b-sha")
		expectHeld("b-sha", "ManualRollback")
		expectNewCommitDeploys()
	})

	// Catches a rollout still in progress resuming after a manual rollback.
	It("holds a rollout in progress a manual rollback replaces", func() {
		hook := customObject("Widget", "migrate", "v1")
		hook.SetAnnotations(map[string]string{"sync.kuvryn.io/hook": "post-sync"})
		r.Renderers = commitRenderer(map[string][]unstructured.Unstructured{"b-sha": {hook}})
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		Expect(revisionsOf("b-sha")[0].Status.Phase).To(Equal(corev1alpha1.RevisionPhaseObserving))

		requestRollback("a-sha", "b-sha")
		reconcileOnce()
		expectHolding("a-sha", "b-sha")
		expectHeld("b-sha", "ManualRollback")
	})

	// Catches a rollback to a held Revision that stayed stuck: the request is
	// an explicit wish to deploy it, so it lifts the hold.
	It("lifts a hold when rolling back to the held Revision", func() {
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		requestRollback("a-sha", "b-sha")
		reconcileOnce()
		expectHeld("b-sha", "ManualRollback")

		requestRollback("b-sha", "b-sha")
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"))
		Expect(application().GetAnnotations()).NotTo(HaveKey(corev1alpha1.RollbackRevisionAnnotation))
		for _, rev := range revisionsOf("b-sha") {
			Expect(heldBy(&rev)).To(BeNil())
		}
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"))
		Expect(ready().Status).To(Equal(metav1.ConditionTrue))
		expectNewCommitDeploys()
	})

	// Catches a rollback request pinning the Application to a target that
	// cannot be deployed, so no new commit ever deployed either.
	It("abandons a rollback whose target fails for good", func() {
		invalid := unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap"}}
		r.Renderers = commitRenderer(map[string][]unstructured.Unstructured{"invalid-sha": {invalid}})
		recorder := record.NewFakeRecorder(100)
		r.Recorder = recorder
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()

		requestRollback("invalid-sha", "b-sha")
		reconcileOnce()
		Expect(application().GetAnnotations()).NotTo(HaveKey(corev1alpha1.RollbackRevisionAnnotation))
		Expect(drainEvents(recorder)).To(ContainElement(And(HavePrefix("Warning RollbackAbandoned"), ContainSubstring("ValidationFailure"))))
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"))
		Expect(ready().Status).To(Equal(metav1.ConditionTrue))
		Expect(revisionsOf("b-sha")[0].Status.Conditions).NotTo(ContainElement(HaveField("Type", corev1alpha1.RolledBackCondition)))
		expectNewCommitDeploys()
	})

	// Catches a rollback pinning the Application once its target's retries
	// are used up.
	It("abandons a rollback once its target's retries run out", func() {
		r.Renderers = commitRenderer(nil, "broken-sha")
		recorder := record.NewFakeRecorder(100)
		r.Recorder = recorder
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()

		requestRollback("broken-sha", "b-sha")
		reconcileOnce()
		Expect(application().GetAnnotations()).To(HaveKey(corev1alpha1.RollbackRevisionAnnotation), "a retryable failure abandoned the rollback")
		reconcileOnce()
		Expect(application().GetAnnotations()).NotTo(HaveKey(corev1alpha1.RollbackRevisionAnnotation))
		Expect(drainEvents(recorder)).To(ContainElement(And(HavePrefix("Warning RollbackAbandoned"), ContainSubstring("maxAttempts"))))
		expectNewCommitDeploys()
	})

	// Catches a transient failure, a failed fetch or a retryable rollout
	// failure, abandoning a rollback that would then succeed.
	It("keeps a rollback through transient failures", func() {
		attempts := int32(3)
		configured := application()
		configured.Spec.Strategy.FailurePolicy.MaxAttempts = &attempts
		Expect(k8sClient.Update(ctx, configured)).To(Succeed())
		unreachable := map[string]bool{}
		r.SourceResolver = namedWorkspaceResolver{revision: &source, unreachable: unreachable}
		failing := true
		r.Renderers = func(corev1alpha1.RenderType) (renderer.Renderer, error) {
			return renderFunc(func(input renderer.Input) ([]unstructured.Unstructured, error) {
				commit := strings.TrimPrefix(input.Workspace, "/tmp/kuvryn-sync-workspace-")
				if commit == "a-sha" && failing && application().GetAnnotations()[corev1alpha1.RollbackRevisionAnnotation] != "" {
					return nil, errors.New("chart pull failed")
				}
				return []unstructured.Unstructured{configMapObject("", commit)}, nil
			}), nil
		}
		recorder := record.NewFakeRecorder(100)
		r.Recorder = recorder
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()

		requestRollback("a-sha", "b-sha")
		unreachable["a-sha"] = true
		result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(rollbackSourceRetry), "a failed fetch left the rollback without a scheduled retry")
		Expect(application().GetAnnotations()).To(HaveKey(corev1alpha1.RollbackRevisionAnnotation), "a failed fetch abandoned the rollback")
		unreachable["a-sha"] = false
		reconcileOnce()
		Expect(application().GetAnnotations()).To(HaveKey(corev1alpha1.RollbackRevisionAnnotation), "a retryable render failure abandoned the rollback")
		reconcileOnce()
		Expect(application().GetAnnotations()).To(HaveKey(corev1alpha1.RollbackRevisionAnnotation), "the backoff abandoned the rollback")
		Expect(drainEvents(recorder)).NotTo(ContainElement(HavePrefix("Warning RollbackAbandoned")))

		failing = false
		target := revisionsOf("a-sha")[0]
		target.Status.CompletedAt = &metav1.Time{Time: time.Now().Add(-time.Hour)}
		Expect(k8sClient.Status().Update(ctx, &target)).To(Succeed())
		reconcileOnce()
		expectHolding("a-sha", "b-sha")
	})

	// Catches a second rollback while one is pending holding the first
	// rollback's target: a request without its source rolls back from what
	// the spec resolves to, not from status.desiredRevision.
	It("holds the spec's commit when a second rollback replaces a pending one", func() {
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		source = "x-sha"
		reconcileOnce()
		manual := application()
		manual.Spec.Sync.Automatic = false
		Expect(k8sClient.Update(ctx, manual)).To(Succeed())
		requestRollback("b-sha", "x-sha")
		reconcileOnce()
		Expect(application().Status.DesiredRevision).To(Equal("b-sha"), "the pending rollback should wait for approval")

		second := application()
		setRollbackRequest(second, rollbackRequest{target: "a-sha"})
		delete(second.Annotations, corev1alpha1.RollbackFromAnnotation)
		delete(second.Annotations, corev1alpha1.RollbackKindAnnotation)
		Expect(k8sClient.Update(ctx, second)).To(Succeed())
		reconcileOnce()
		Expect(application().GetAnnotations()).To(HaveKeyWithValue(corev1alpha1.RollbackFromAnnotation, "x-sha"))

		target := revisionsOf("a-sha")[0]
		approving := application()
		approving.Annotations[corev1alpha1.ApprovedRevisionAnnotation] = target.Name
		approving.Annotations[corev1alpha1.ApprovedByAnnotation] = "alice@example.com"
		approving.Annotations[corev1alpha1.ApprovedAtAnnotation] = time.Now().UTC().Format(time.RFC3339)
		approving.Annotations[corev1alpha1.ApprovedDigestAnnotation] = target.Status.Plan.Digest
		Expect(k8sClient.Update(ctx, approving)).To(Succeed())
		reconcileOnce()
		Expect(deployed()).To(Equal("a-sha"))
		expectHeld("x-sha", "ManualRollback")
		for _, rev := range revisionsOf("b-sha") {
			Expect(heldBy(&rev)).To(BeNil(), "the pending rollback's target was held")
		}
	})

	// Catches a hold that froze the Application when its held commit was
	// deployed again under another identity and the change was reverted.
	It("lifts a hold when the held commit is deployed again", func() {
		adopting := application()
		adopting.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyAdopt
		Expect(k8sClient.Update(ctx, adopting)).To(Succeed())
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		requestRollback("a-sha", "b-sha")
		reconcileOnce()
		expectHolding("a-sha", "b-sha")

		moved := application()
		path := moved.Spec.Source.Path
		moved.Spec.Source.Path = "apps/elsewhere"
		Expect(k8sClient.Update(ctx, moved)).To(Succeed())
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"), "an identity change should end the hold")

		reverted := application()
		reverted.Spec.Source.Path = path
		Expect(k8sClient.Update(ctx, reverted)).To(Succeed())
		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, live)).To(Succeed())
		live.Data["key"] = "edited-by-hand"
		Expect(k8sClient.Update(ctx, live)).To(Succeed())
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"), "the reverted Application was not reconciled")
		reconcileOnce()
		app := application()
		Expect(app.Status.Sync.State).To(Equal(corev1alpha1.SyncStateSynced))
		Expect(ready().Status).To(Equal(metav1.ConditionTrue))
		for _, rev := range revisionsOf("b-sha") {
			Expect(heldBy(&rev)).To(BeNil())
		}
	})

	// Catches drift of the held rollback target hidden as OutOfSync.
	It("reports drift of the deployed revision while held", func() {
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		requestRollback("a-sha", "b-sha")
		reconcileOnce()
		expectHolding("a-sha", "b-sha")

		observing := application()
		observing.Spec.Sync.SelfHeal = false
		Expect(k8sClient.Update(ctx, observing)).To(Succeed())
		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, live)).To(Succeed())
		live.Data["key"] = "edited-by-hand"
		Expect(k8sClient.Update(ctx, live)).To(Succeed())
		reconcileOnce()
		Expect(deployed()).To(Equal("edited-by-hand"))
		Expect(application().Status.Sync.State).To(Equal(corev1alpha1.SyncStateDrifted))
		Expect(ready().Reason).To(Equal("RolledBack"))
	})

	// Catches history retention deleting the held Revision, after which the
	// held commit deployed again.
	It("never deletes a held Revision for history", func() {
		limited := application()
		limited.Spec.History.Limit = ptr.To[int32](1)
		limited.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyAdopt
		Expect(k8sClient.Update(ctx, limited)).To(Succeed())
		reconcileOnce()
		// Retention keeps the newest Revisions by creation time, which has
		// second precision.
		time.Sleep(1100 * time.Millisecond)
		source = "b-sha"
		reconcileOnce()
		requestRollback("a-sha", "b-sha")
		reconcileOnce()
		expectHeld("b-sha", "ManualRollback")
		// Retention runs after an apply. Once the rollback target is newer
		// than the held Revision, as when it is created again, a self-heal
		// must not delete the held one either.
		time.Sleep(1100 * time.Millisecond)
		reconcileOnce()
		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, live)).To(Succeed())
		live.Data["key"] = "edited-by-hand"
		Expect(k8sClient.Update(ctx, live)).To(Succeed())
		reconcileOnce()
		expectHeld("b-sha", "ManualRollback")
		expectHolding("a-sha", "b-sha")
	})

	// Catches a hold that froze the Application: the deployed rollback target
	// is still reconciled, self-healed, and its health observed.
	It("keeps reconciling the deployed revision while held", func() {
		// A hand edit takes over the field it changes; adopt lets self-heal
		// take it back.
		adopting := application()
		adopting.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyAdopt
		Expect(k8sClient.Update(ctx, adopting)).To(Succeed())
		widget := customObject("Widget", "health", "v1")
		r.Renderers = commitRenderer(map[string][]unstructured.Unstructured{"a-sha": {widget}})
		reconcileOnce()
		setWidget(map[string]any{"type": "Ready", "status": "True"})
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		Expect(deployed()).To(Equal("b-sha"))

		requestRollback("a-sha", "b-sha")
		reconcileOnce()
		setWidget(map[string]any{"type": "Ready", "status": "True"})
		reconcileOnce()
		expectHolding("a-sha", "b-sha")

		By("self-healing drift and observing that the target degraded")
		live := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: "payments"}, live)).To(Succeed())
		live.Data["key"] = "edited-by-hand"
		Expect(k8sClient.Update(ctx, live)).To(Succeed())
		setWidget(map[string]any{"type": "Stalled", "status": "True", "message": "disk full"})
		reconcileOnce()
		Expect(deployed()).To(Equal("a-sha"), "drift was not self-healed while held")
		app := application()
		Expect(app.Status.Health.State).To(Equal(corev1alpha1.HealthStateDegraded))
		Expect(app.Status.DesiredRevision).To(Equal("b-sha"))
		Expect(app.Status.DeployedRevision).To(Equal("a-sha"))
	})

	// Catches marking that is not retried: when a write conflicts, the
	// request must survive so the next reconcile marks again.
	It("retries holding after a conflict", func() {
		watching, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
		Expect(err).NotTo(HaveOccurred())
		conflicted := false
		r.Client = interceptor.NewClient(watching, interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if rev, ok := obj.(*corev1alpha1.Revision); ok && !conflicted && heldBy(rev) != nil {
					conflicted = true
					return apierrors.NewConflict(schema.GroupResource{Group: "sync.kuvryn.io", Resource: "revisions"}, rev.Name, errors.New("stale"))
				}
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		})
		reconcileOnce()
		source = "b-sha"
		reconcileOnce()
		requestRollback("a-sha", "b-sha")
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "err = %v", err)
		Expect(application().GetAnnotations()).To(HaveKeyWithValue(corev1alpha1.RollbackFromAnnotation, "b-sha"))

		reconcileOnce()
		expectHeld("b-sha", "ManualRollback")
		expectHolding("a-sha", "b-sha")
	})

	// Catches a status write from a stale copy overwriting newer status.
	It("refuses a Revision status write from a stale copy", func() {
		reconcileOnce()
		stale := &revisionsOf("a-sha")[0]
		fresh := stale.DeepCopy()
		fresh.Status.Attempts = 7
		Expect(k8sClient.Status().Update(ctx, fresh)).To(Succeed())
		stale.Status.Attempts = 1
		Expect(apierrors.IsConflict(r.updateRevisionStatus(ctx, stale))).To(BeTrue())
		Expect(revisionsOf("a-sha")[0].Status.Attempts).To(Equal(int32(7)))
	})
})

// Catches an approval deploying a Revision a rollback replaced.
func TestManualApprovalIgnoresAHeldRevision(t *testing.T) {
	revision := &corev1alpha1.Revision{ObjectMeta: metav1.ObjectMeta{Name: "payments-b"}}
	revision.Status.Plan.Digest = "digest"
	app := &corev1alpha1.Application{}
	app.SetAnnotations(map[string]string{
		corev1alpha1.ApprovedRevisionAnnotation: "payments-b",
		corev1alpha1.ApprovedByAnnotation:       "alice@example.com",
		corev1alpha1.ApprovedAtAnnotation:       time.Now().UTC().Format(time.RFC3339),
		corev1alpha1.ApprovedDigestAnnotation:   "digest",
	})
	if approval, _ := manualApproval(app, revision, rolloutNotStarted); approval == nil {
		t.Fatal("the approval of a Revision that is not held was ignored")
	}
	revision.Status.Conditions = []metav1.Condition{{Type: corev1alpha1.RolledBackCondition, Status: metav1.ConditionTrue, Reason: manualRollbackReason}}
	if approval, _ := manualApproval(app, revision, rolloutNotStarted); approval != nil {
		t.Fatalf("a held Revision was approved: %+v", approval)
	}
}

// Catches recordRollbackIntent writing the Application on every reconcile
// although the request already records everything.
func TestRecordRollbackIntentWritesOnlyChanges(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	app := &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"}}
	setRollbackRequest(app, rollbackRequest{target: "a-sha", from: "b-sha", kind: corev1alpha1.RollbackKindManual})
	updates := 0
	c := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme).WithObjects(app).Build(), interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			updates++
			return c.Update(ctx, obj, opts...)
		},
	})
	r := &ApplicationReconciler{Client: c}
	live := &corev1alpha1.Application{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(app), live); err != nil {
		t.Fatal(err)
	}
	if _, err := r.recordRollbackIntent(context.Background(), live, rollbackRequestOf(live)); err != nil || updates != 0 {
		t.Fatalf("a complete request was written %d times: %v", updates, err)
	}
	delete(live.Annotations, corev1alpha1.RollbackKindAnnotation)
	if req, err := r.recordRollbackIntent(context.Background(), live, rollbackRequestOf(live)); err != nil || updates != 1 || req.kind != corev1alpha1.RollbackKindManual {
		t.Fatalf("a request without its kind: %d writes, %+v, %v", updates, req, err)
	}
}

// Catches a Revision labelled with the Application's name but referring to
// another Application being held, lifted, or deleted with it.
func TestApplicationRevisionsRequireTheApplicationRef(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	revision := func(name, application string) *corev1alpha1.Revision {
		return &corev1alpha1.Revision{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{"sync.kuvryn.io/application": "payments"}},
			Spec:       corev1alpha1.RevisionSpec{ApplicationRef: corev1alpha1.LocalObjectReference{Name: application}},
		}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(revision("payments-a", "payments"), revision("search-a", "search")).Build()
	r := &ApplicationReconciler{Client: c}
	revisions, err := r.applicationRevisions(context.Background(), &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"}})
	if err != nil || len(revisions) != 1 || revisions[0].Name != "payments-a" {
		t.Fatalf("revisions = %+v, %v", revisions, err)
	}
}

// Catches a rollback approving what nobody requested: a failure policy's
// rollback, a request without the requester the webhook records, another
// Revision than the one the requester chose, one whose desired state
// changed since, or a held one; and a rollout in progress keeping an
// approval for a desired state that changed.
func TestRollbackApprovalNeedsAPersonsRequestForTheTarget(t *testing.T) {
	at := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	app := func(kind string, annotations map[string]string) *corev1alpha1.Application {
		a := &corev1alpha1.Application{}
		setRollbackRequest(a, rollbackRequest{target: "a-sha", from: "b-sha", kind: kind})
		for k, v := range annotations {
			a.Annotations[k] = v
		}
		return a
	}
	requester := map[string]string{
		corev1alpha1.RollbackTargetRevisionAnnotation: "payments-a",
		corev1alpha1.RollbackRequestedByAnnotation:    "alice@example.com",
		corev1alpha1.RollbackRequestedAtAnnotation:    at.Format(time.RFC3339),
		corev1alpha1.RollbackTargetHashAnnotation:     revisionid.Binding("hash", &corev1alpha1.Application{}),
	}
	target := func() *corev1alpha1.Revision {
		rev := &corev1alpha1.Revision{ObjectMeta: metav1.ObjectMeta{Name: "payments-a"}}
		rev.Spec.Source.Revision, rev.Spec.DesiredStateHash, rev.Status.Plan.Digest = "a-sha", "hash", "digest-now"
		return rev
	}

	manual := app(corev1alpha1.RollbackKindManual, requester)
	approval, changed := rollbackApproval(manual, rollbackRequestOf(manual), target(), rolloutComplete)
	if approval == nil || changed || approval.ApprovedBy != "alice@example.com" || !approval.ApprovedAt.Time.Equal(at) || approval.PlanDigest != "digest-now" || approval.DesiredStateHash != "hash" {
		t.Fatalf("approval = %+v, changed = %v; want alice's, for the current plan", approval, changed)
	}

	automatic := app(corev1alpha1.RollbackKindAutomatic, requester)
	held := target()
	held.Status.Conditions = []metav1.Condition{{Type: corev1alpha1.RolledBackCondition, Status: metav1.ConditionTrue, Reason: manualRollbackReason}}
	unplanned := target()
	unplanned.Status.Plan.Digest = ""
	anonymous := app(corev1alpha1.RollbackKindManual, nil)
	unbound := app(corev1alpha1.RollbackKindManual, map[string]string{
		corev1alpha1.RollbackRequestedByAnnotation: "alice@example.com",
		corev1alpha1.RollbackRequestedAtAnnotation: at.Format(time.RFC3339),
	})
	for name, tc := range map[string]struct {
		app *corev1alpha1.Application
		rev *corev1alpha1.Revision
	}{
		"automatic":          {automatic, target()},
		"no requester":       {anonymous, target()},
		"no chosen Revision": {unbound, target()},
		"a held Revision":    {manual, held},
		"no plan yet":        {manual, unplanned},
		"no rollback at all": {&corev1alpha1.Application{}, target()},
	} {
		if approval, changed := rollbackApproval(tc.app, rollbackRequestOf(tc.app), tc.rev, rolloutNotStarted); approval != nil || changed {
			t.Errorf("%s: approved %+v, changed %v", name, approval, changed)
		}
	}

	// The spec changed since the request: the controller built another
	// Revision of the same commit, or the chosen one renders differently.
	rebuilt := target()
	rebuilt.Name = "payments-a2"
	rendersDifferently := target()
	rendersDifferently.Spec.DesiredStateHash = "hash-after-values-change"
	for name, rev := range map[string]*corev1alpha1.Revision{"another Revision": rebuilt, "another desired state": rendersDifferently} {
		if approval, changed := rollbackApproval(manual, rollbackRequestOf(manual), rev, rolloutNotStarted); approval != nil || !changed {
			t.Errorf("%s: approved %+v, changed %v; want no approval and the target reported changed", name, approval, changed)
		}
	}

	// The sync policy or rollout strategy changed since the request: the
	// requester did not choose to apply the target that way.
	for name, change := range map[string]func(*corev1alpha1.Application){
		"prune":          func(a *corev1alpha1.Application) { a.Spec.Sync.Prune = true },
		"conflictPolicy": func(a *corev1alpha1.Application) { a.Spec.Sync.ConflictPolicy = corev1alpha1.ConflictPolicyAdopt },
		"selfHeal":       func(a *corev1alpha1.Application) { a.Spec.Sync.SelfHeal = true },
		"strategy": func(a *corev1alpha1.Application) {
			a.Spec.Strategy.FailurePolicy.Action = corev1alpha1.FailureActionRollback
		},
	} {
		policy := app(corev1alpha1.RollbackKindManual, requester)
		change(policy)
		if approval, changed := rollbackApproval(policy, rollbackRequestOf(policy), target(), rolloutNotStarted); approval != nil || !changed {
			t.Errorf("%s changed: approved %+v, changed %v; want no approval and the target reported changed", name, approval, changed)
		}
	}

	// A rollout in progress keeps the approval it acted on, although
	// applying a group changed the plan, but never for a new desired state.
	inProgress := target()
	inProgress.Status.Approval = &corev1alpha1.RevisionApproval{ApprovedBy: "alice@example.com", ApprovedAt: metav1.NewTime(at), PlanDigest: "digest-first", DesiredStateHash: "hash"}
	if approval, _ := rollbackApproval(manual, rollbackRequestOf(manual), inProgress, rolloutInProgress); approval == nil || approval.PlanDigest != "digest-first" {
		t.Fatalf("approval = %+v; want the one the rollout acted on", approval)
	}
	inProgress.Spec.DesiredStateHash = "hash-after-values-change"
	if approval, changed := rollbackApproval(manual, rollbackRequestOf(manual), inProgress, rolloutInProgress); approval != nil || !changed {
		t.Fatalf("a rollout in progress approved a new desired state: %+v, changed %v", approval, changed)
	}
}

// Catches rollback requests recorded while admission webhooks were off
// passing unnoticed once they are on again: the webhook keeps an unchanged
// request's record, so the manager names every Application carrying a
// pending request's requester at startup.
func TestPendingRollbackRequestsAreListedAtStartup(t *testing.T) {
	scheme := k8sruntime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	app := func(namespace, name string, annotations map[string]string) *corev1alpha1.Application {
		return &corev1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Annotations: annotations}}
	}
	pending := map[string]string{
		corev1alpha1.RollbackRevisionAnnotation:    "a-sha",
		corev1alpha1.RollbackRequestedByAnnotation: "mallory@example.com",
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		app("payments", "api", pending),
		app("payments", "web", map[string]string{corev1alpha1.RollbackRevisionAnnotation: "a-sha"}),
		app("search", "index", nil),
		app("billing", "ledger", pending),
	).Build()
	got, err := pendingRollbackRequests(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"billing/ledger", "payments/api"}
	if !slices.Equal(got, want) {
		t.Fatalf("pending requests = %v, want %v", got, want)
	}
}
