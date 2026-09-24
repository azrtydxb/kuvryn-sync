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
	"strings"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
)

// RolledBackCondition is True on a Revision a completed rollback replaced.
// Solder does not deploy a held Revision again: the Application stays on the
// rollback target until a new commit, or a new Revision identity, arrives.
const RolledBackCondition = corev1alpha1.RolledBackCondition

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
	application.SetAnnotations(annotations)
}

// recordRollbackIntent completes a rollback request that does not say what it
// rolls back from, such as one set by hand, with the revision the Application
// desires now, before reconciling the target moves status.desiredRevision.
func (r *ApplicationReconciler) recordRollbackIntent(ctx context.Context, application *corev1alpha1.Application, req rollbackRequest) (rollbackRequest, error) {
	if req.from != "" && req.kind != "" {
		return req, nil
	}
	if req.from == "" {
		req.from = application.Status.DesiredRevision
	}
	if req.kind == "" {
		req.kind = corev1alpha1.RollbackKindManual
	}
	setRollbackRequest(application, req)
	return req, r.updateKeepingStatus(ctx, application)
}

// heldBy returns the RolledBack condition holding revision, if any.
func heldBy(revision *corev1alpha1.Revision) *metav1.Condition {
	condition := apimeta.FindStatusCondition(revision.Status.Conditions, RolledBackCondition)
	if condition == nil || condition.Status != metav1.ConditionTrue {
		return nil
	}
	return condition
}

// holdMessage is the Application Ready message while a rollback holds.
func holdMessage(manual bool) string {
	if manual {
		return "Application was rolled back manually to an earlier Revision; a new commit deploys again"
	}
	return "Application was rolled back to an earlier Revision after a failure; a new commit deploys again"
}

// holdReplaced marks every Revision of the source revision a completed
// rollback replaced, in any phase, as Failed and held, so the Application
// stays on the rollback target. The target's own Revisions are left alone.
// It is idempotent: a retry after a conflict marks the same Revisions.
func (r *ApplicationReconciler) holdReplaced(ctx context.Context, application *corev1alpha1.Application, target *corev1alpha1.Revision, req rollbackRequest) error {
	if req.from == "" || req.from == target.Spec.Source.Revision {
		return nil
	}
	var list corev1alpha1.RevisionList
	if err := r.List(ctx, &list, client.InNamespace(application.Namespace), client.MatchingLabels{"solder.io/application": application.Name}); err != nil {
		return err
	}
	how := "manually"
	condition := metav1.Condition{Type: RolledBackCondition, Status: metav1.ConditionTrue, Reason: manualRollbackReason}
	if req.automatic() {
		how, condition.Reason = "after a failure", automaticRollbackReason
	}
	condition.Message = fmt.Sprintf("Rolled back %s to source revision %s", how, target.Spec.Source.Revision)
	failure := corev1alpha1.RevisionFailure{Reason: "RolledBack", Message: condition.Message, Retryable: false}
	for i := range list.Items {
		replaced := &list.Items[i]
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

// liftHold clears the hold on a Revision an explicit rollback targets: the
// user asked to deploy exactly it.
func (r *ApplicationReconciler) liftHold(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision) error {
	apimeta.RemoveStatusCondition(&revision.Status.Conditions, RolledBackCondition)
	revision.Status.Failure = nil
	revision.Status.Phase = corev1alpha1.RevisionPhasePending
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return err
	}
	r.event(application, corev1.EventTypeNormal, "RollbackHoldLifted", fmt.Sprintf("Rollback to source revision %s lifted its hold", revision.Spec.Source.Revision))
	return nil
}

// abandonRollback gives up a rollback whose target cannot be deployed, so the
// request never pins the Application: the next reconcile deploys its desired
// revision again. A failure policy's failing Revision, left RollingBack, is
// failed so retry limits apply to it.
func (r *ApplicationReconciler) abandonRollback(ctx context.Context, application *corev1alpha1.Application, req rollbackRequest, why string) error {
	if req.automatic() && req.from != "" {
		var list corev1alpha1.RevisionList
		if err := r.List(ctx, &list, client.InNamespace(application.Namespace), client.MatchingLabels{"solder.io/application": application.Name}); err != nil {
			return err
		}
		for i := range list.Items {
			if rev := &list.Items[i]; rev.Spec.Source.Revision == req.from && rev.Status.Phase == corev1alpha1.RevisionPhaseRollingBack {
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
// holds, the desired revision is the held one and is not deployed, so sync is
// OutOfSync whatever reconciling the deployed revision found.
func (r *ApplicationReconciler) updateApplicationStatus(ctx context.Context, application *corev1alpha1.Application) error {
	if hold := holdFrom(ctx); hold != nil {
		application.Status.DesiredRevision = hold.desired
		switch application.Status.Sync.State {
		case corev1alpha1.SyncStateSynced, corev1alpha1.SyncStateDrifted:
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
