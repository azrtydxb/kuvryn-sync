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

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
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
	application.SetAnnotations(annotations)
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
	if err := r.List(ctx, &list, client.InNamespace(application.Namespace), client.MatchingLabels{"solder.io/application": application.Name}); err != nil {
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
