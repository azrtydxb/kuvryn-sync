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
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/revisionid"
)

// rollbackSourceRetry is how often a rollback whose target cannot be fetched
// is retried.
const rollbackSourceRetry = time.Minute

const (
	// manualRollbackReason is the RolledBack reason for a rollback a user
	// requested.
	manualRollbackReason = "ManualRollback"
	// automaticRollbackReason is the RolledBack reason for a rollback a
	// failure policy started.
	automaticRollbackReason = "RollbackCompleted"
)

// rollbackRequest is the rollback recorded in an Application's annotations.
type rollbackRequest struct {
	target, from, kind string
}

func rollbackRequestOf(application *corev1alpha1.Application) rollbackRequest {
	annotations := application.GetAnnotations()
	return rollbackRequest{
		target: strings.TrimSpace(annotations[corev1alpha1.RollbackRevisionAnnotation]),
		from:   strings.TrimSpace(annotations[corev1alpha1.RollbackFromAnnotation]),
		kind:   annotations[corev1alpha1.RollbackKindAnnotation],
	}
}

func (req rollbackRequest) active() bool { return req.target != "" }

// automatic reports a rollback a failure policy started. A request without a
// kind, such as a hand-set annotation, is manual.
func (req rollbackRequest) automatic() bool { return req.kind == corev1alpha1.RollbackKindAutomatic }

func setRollbackRequest(application *corev1alpha1.Application, req rollbackRequest) {
	metav1.SetMetaDataAnnotation(&application.ObjectMeta, corev1alpha1.RollbackRevisionAnnotation, req.target)
	metav1.SetMetaDataAnnotation(&application.ObjectMeta, corev1alpha1.RollbackFromAnnotation, req.from)
	metav1.SetMetaDataAnnotation(&application.ObjectMeta, corev1alpha1.RollbackKindAnnotation, req.kind)
}

func clearRollbackRequest(application *corev1alpha1.Application) {
	annotations := application.GetAnnotations()
	delete(annotations, corev1alpha1.RollbackRevisionAnnotation)
	delete(annotations, corev1alpha1.RollbackFromAnnotation)
	delete(annotations, corev1alpha1.RollbackKindAnnotation)
	delete(annotations, corev1alpha1.RollbackTargetRevisionAnnotation)
	delete(annotations, corev1alpha1.RollbackRequestedByAnnotation)
	delete(annotations, corev1alpha1.RollbackRequestedAtAnnotation)
	delete(annotations, corev1alpha1.RollbackTargetHashAnnotation)
	application.SetAnnotations(annotations)
}

// rollbackApproval approves the plan of a manual rollback's target on an
// Application with manual sync: requesting the rollback is the decision to
// deploy the Revision the requester chose. The admission webhook records on
// the request who made it, when, which Revision they chose and that
// Revision's desired-state hash at the time. The approval is recorded under
// that requester and time, for the digest of the plan this reconcile
// applies, and only while revision is the chosen Revision with the desired
// state it had then. A failure policy's rollback, a request without a
// recorded requester or chosen Revision, and a held Revision are not
// approved.
//
// changed reports a person's request whose target is no longer what they
// chose: the spec changed since, so the controller built another Revision
// of the commit, or the chosen one now renders another desired state, such
// as after a Helm values change. It must be approved with ksync sync.
//
// Applying one group of a rollout changes the plan for the next, so while
// the rollout is in progress the approval already recorded for this request
// stands, as manualApproval keeps it; the desired state must still be the
// chosen one.
func rollbackApproval(application *corev1alpha1.Application, req rollbackRequest, revision *corev1alpha1.Revision, rollout rolloutState) (approval *corev1alpha1.RevisionApproval, changed bool) {
	if !req.active() || req.automatic() || revision.Spec.Source.Revision != req.target || heldBy(revision) != nil {
		return nil, false
	}
	annotations := application.GetAnnotations()
	chosen := annotations[corev1alpha1.RollbackTargetRevisionAnnotation]
	chosenHash := annotations[corev1alpha1.RollbackTargetHashAnnotation]
	requestedBy := annotations[corev1alpha1.RollbackRequestedByAnnotation]
	requestedAt, err := time.Parse(time.RFC3339, annotations[corev1alpha1.RollbackRequestedAtAnnotation])
	if chosen == "" || chosenHash == "" || requestedBy == "" || err != nil {
		return nil, false
	}
	if revision.Name != chosen || revisionid.Binding(revision.Spec.DesiredStateHash, application) != chosenHash {
		return nil, true
	}
	if revision.Status.Plan.Digest == "" {
		return nil, false
	}
	approval = &corev1alpha1.RevisionApproval{
		ApprovedBy: requestedBy, ApprovedAt: metav1.NewTime(requestedAt),
		PlanDigest: revision.Status.Plan.Digest, DesiredStateHash: revision.Spec.DesiredStateHash,
	}
	if recorded := revision.Status.Approval; recorded != nil && rollout == rolloutInProgress &&
		recorded.ApprovedBy == approval.ApprovedBy && recorded.ApprovedAt.Equal(&approval.ApprovedAt) &&
		recorded.DesiredStateHash == approval.DesiredStateHash {
		return recorded, false
	}
	return approval, false
}

// sameApproval reports whether two approvals record the same decision.
func sameApproval(a, b *corev1alpha1.RevisionApproval) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ApprovedBy == b.ApprovedBy && a.ApprovedAt.Equal(&b.ApprovedAt) && a.PlanDigest == b.PlanDigest && a.DesiredStateHash == b.DesiredStateHash
}

// recordRollbackIntent records a request's source and kind when the request
// did not, such as one set by hand. It writes only when something changed.
func (r *ApplicationReconciler) recordRollbackIntent(ctx context.Context, application *corev1alpha1.Application, req rollbackRequest) (rollbackRequest, error) {
	if req.kind == "" {
		req.kind = corev1alpha1.RollbackKindManual
	}
	if req == rollbackRequestOf(application) {
		return req, nil
	}
	setRollbackRequest(application, req)
	return req, r.updateKeepingStatus(ctx, application)
}

// applicationRevisions lists the Revisions of application: labelled with its
// name and referring to it.
func (r *ApplicationReconciler) applicationRevisions(ctx context.Context, application *corev1alpha1.Application) ([]corev1alpha1.Revision, error) {
	var list corev1alpha1.RevisionList
	if err := r.List(ctx, &list, client.InNamespace(application.Namespace), client.MatchingLabels{"sync.kuvryn.io/application": application.Name}); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list.Items, func(rev corev1alpha1.Revision) bool {
		return rev.Spec.ApplicationRef.Name != application.Name
	}), nil
}

// heldBy returns the RolledBack condition holding revision, if any.
func heldBy(revision *corev1alpha1.Revision) *metav1.Condition {
	condition := apimeta.FindStatusCondition(revision.Status.Conditions, corev1alpha1.RolledBackCondition)
	if condition == nil || condition.Status != metav1.ConditionTrue {
		return nil
	}
	return condition
}

// rollbackWording is the RolledBack reason for a manual or automatic rollback
// and how it happened, for messages.
func rollbackWording(manual bool) (reason, how string) {
	if manual {
		return manualRollbackReason, "manually"
	}
	return automaticRollbackReason, "after a failure"
}

// holdMessage is the Application Ready message while a rollback holds.
func holdMessage(manual bool) string {
	_, how := rollbackWording(manual)
	return "Application was rolled back " + how + " to an earlier Revision; a new commit deploys again"
}

// holdReplaced marks every Revision of the source revision a completed
// rollback replaced, in any phase, as Failed and held, so the Application
// stays on the rollback target. The target's own Revisions are left alone.
// It is idempotent: a retry after a conflict marks the same Revisions.
func (r *ApplicationReconciler) holdReplaced(ctx context.Context, application *corev1alpha1.Application, target *corev1alpha1.Revision, req rollbackRequest) error {
	if req.from == "" || req.from == target.Spec.Source.Revision {
		return nil
	}
	revisions, err := r.applicationRevisions(ctx, application)
	if err != nil {
		return err
	}
	reason, how := rollbackWording(!req.automatic())
	condition := metav1.Condition{
		Type: corev1alpha1.RolledBackCondition, Status: metav1.ConditionTrue, Reason: reason,
		Message: fmt.Sprintf("Rolled back %s to source revision %s", how, target.Spec.Source.Revision),
	}
	failure := corev1alpha1.RevisionFailure{Reason: "RolledBack", Message: condition.Message, Retryable: false}
	for i := range revisions {
		replaced := &revisions[i]
		if replaced.Name == target.Name || replaced.Spec.Source.Revision != req.from {
			continue
		}
		if held := heldBy(replaced); held != nil && held.Reason == condition.Reason && replaced.Status.Phase == corev1alpha1.RevisionPhaseFailed {
			continue
		}
		replaced.Status.Phase = corev1alpha1.RevisionPhaseFailed
		if replaced.Status.Failure == nil {
			replaced.Status.Failure = failure.DeepCopy()
		}
		condition.ObservedGeneration = replaced.Generation
		apimeta.SetStatusCondition(&replaced.Status.Conditions, condition)
		if err := r.updateRevisionStatus(ctx, replaced); err != nil {
			return err
		}
	}
	return nil
}

// liftHold clears the hold on a Revision, because of why, so it deploys
// again: an explicit rollback to it, or its commit running again.
func (r *ApplicationReconciler) liftHold(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, why string) error {
	apimeta.RemoveStatusCondition(&revision.Status.Conditions, corev1alpha1.RolledBackCondition)
	revision.Status.Failure = nil
	revision.Status.Phase = corev1alpha1.RevisionPhasePending
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return err
	}
	r.event(application, corev1.EventTypeNormal, "RollbackHoldLifted", fmt.Sprintf("Hold on source revision %s lifted by %s", revision.Spec.Source.Revision, why))
	return nil
}

// abandonRollback gives up a rollback whose target cannot be deployed, so the
// request never pins the Application: the next reconcile deploys its desired
// revision again. A failure policy's failing Revision, left RollingBack, is
// failed so retry limits apply to it.
func (r *ApplicationReconciler) abandonRollback(ctx context.Context, application *corev1alpha1.Application, req rollbackRequest, why string) error {
	if req.automatic() && req.from != "" {
		revisions, err := r.applicationRevisions(ctx, application)
		if err != nil {
			return err
		}
		for i := range revisions {
			if rev := &revisions[i]; rev.Spec.Source.Revision == req.from && rev.Status.Phase == corev1alpha1.RevisionPhaseRollingBack {
				rev.Status.Phase = corev1alpha1.RevisionPhaseFailed
				if err := r.updateRevisionStatus(ctx, rev); err != nil {
					return err
				}
			}
		}
	}
	clearRollbackRequest(application)
	if err := r.updateKeepingStatus(ctx, application); err != nil {
		return err
	}
	r.event(application, corev1.EventTypeWarning, "RollbackAbandoned", safeMessage(fmt.Errorf("rollback to source revision %s was abandoned: %s", req.target, why), "Rollback was abandoned"))
	return nil
}

type holdKey struct{}

// rollbackHold is set on the context of a reconcile that keeps a held
// Application on its deployed revision.
type rollbackHold struct {
	// desired is the held source revision the Application desires.
	desired string
	manual  bool
}

func withHold(ctx context.Context, hold *rollbackHold) context.Context {
	return context.WithValue(ctx, holdKey{}, hold)
}

func holdFrom(ctx context.Context) *rollbackHold {
	hold, _ := ctx.Value(holdKey{}).(*rollbackHold)
	return hold
}

// updateApplicationStatus writes the Application's status. While a rollback
// holds, the desired revision is the held one and is not deployed, so a
// deployed target that matches its own desired state is OutOfSync; drift of
// the target stays visible as Drifted.
func (r *ApplicationReconciler) updateApplicationStatus(ctx context.Context, application *corev1alpha1.Application) error {
	if hold := holdFrom(ctx); hold != nil {
		application.Status.DesiredRevision = hold.desired
		if application.Status.Sync.State == corev1alpha1.SyncStateSynced {
			application.Status.Sync.State = corev1alpha1.SyncStateOutOfSync
		}
	}
	return r.Status().Update(ctx, application)
}

// reportHold records a hold when there is no other deployed revision to keep
// reconciling.
func (r *ApplicationReconciler) reportHold(ctx context.Context, application *corev1alpha1.Application, hold *rollbackHold, health, state corev1alpha1.HealthState) error {
	application.Status.Health.State, application.Status.State = health, state
	application.Status.Sync.State = corev1alpha1.SyncStateOutOfSync
	setReady(application, metav1.ConditionFalse, "RolledBack", holdMessage(hold.manual))
	return r.updateApplicationStatus(withHold(ctx, hold), application)
}

// RollbackRequestAudit logs, once at startup, every Application carrying a
// pending rollback request with a recorded requester. The admission webhook
// keeps the record of an unchanged request, so one written by hand while
// webhooks were disabled survives turning them on again, and on an
// Application with manual sync it still approves its target. Requests are
// not ignored by age instead: a legitimate rollback whose rollout spans a
// manager restart, such as an upgrade, would then lose its approval.
type RollbackRequestAudit struct {
	// Reader lists Applications before the cache has started.
	Reader client.Reader
}

// Start implements manager.Runnable.
func (a *RollbackRequestAudit) Start(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName("rollback-audit")
	pending, err := pendingRollbackRequests(ctx, a.Reader)
	if err != nil {
		log.Error(err, "Could not list Applications for pending rollback requests")
		return nil
	}
	for _, name := range pending {
		log.Info("Application carries a pending rollback request; if it was recorded while admission webhooks were disabled, its requester was not verified: remove its sync.kuvryn.io/rollback-* annotations and request the rollback again", "application", name)
	}
	return nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable: every
// replica warns.
func (a *RollbackRequestAudit) NeedLeaderElection() bool { return false }

// pendingRollbackRequests names, as namespace/name and sorted, the
// Applications with a pending rollback request that records a requester.
func pendingRollbackRequests(ctx context.Context, reader client.Reader) ([]string, error) {
	var list corev1alpha1.ApplicationList
	if err := reader.List(ctx, &list); err != nil {
		return nil, err
	}
	out := []string{}
	for _, app := range list.Items {
		if rollbackRequestOf(&app).active() && app.Annotations[corev1alpha1.RollbackRequestedByAnnotation] != "" {
			out = append(out, app.Namespace+"/"+app.Name)
		}
	}
	slices.Sort(out)
	return out, nil
}
