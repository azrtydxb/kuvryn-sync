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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/health"
	"github.com/azrtydxb/kuvryn-sync/internal/live"
	"github.com/azrtydxb/kuvryn-sync/internal/ordering"
	"github.com/azrtydxb/kuvryn-sync/internal/resource"
)

// healthRecheck is how soon an Application whose finished rollout is not
// Healthy, or whose health could not be evaluated, is checked again. The
// watches on its workloads report most changes sooner.
func (r *ApplicationReconciler) healthRecheck() time.Duration {
	if r.DriftResyncInterval > 0 {
		return r.DriftResyncInterval
	}
	return 5 * time.Minute
}

// finishedRolloutHealth evaluates the health of a finished rollout's synced
// resources from found, the live state the plan just read, records the
// resource summary and diagnosis, and returns the Application's health with a
// message naming the first cause. A resource that is Degraded, or whose
// diagnosis finds evidence such as a crash loop, makes it Degraded; one only
// on its way makes it Progressing. A resource that no longer exists is
// Degraded: the rollout created it. Hooks are not evaluated, since each runs
// once per rollout.
func (r *ApplicationReconciler) finishedRolloutHealth(ctx context.Context, tenant client.Client, application *corev1alpha1.Application, rendered []unstructured.Unstructured, found live.Result, previousHealth corev1alpha1.HealthState) (corev1alpha1.HealthState, string, error) {
	evaluator, err := r.healthEvaluator(ctx, application)
	if err != nil {
		return "", "", err
	}
	var results []health.Result
	var observed []unstructured.Unstructured
	for _, group := range ordering.Groups(rendered) {
		if group.Stage != ordering.StageSync {
			continue
		}
		for _, obj := range group.Objects {
			id, err := resource.FromObject(obj)
			if err != nil {
				return "", "", err
			}
			current, ok := found.Found[id]
			if !ok {
				results = append(results, health.Result{Resource: id, State: corev1alpha1.HealthStateDegraded, Reason: "Missing", Message: "resource no longer exists"})
				continue
			}
			result, err := evaluator.Evaluate(current)
			if err != nil {
				return "", "", err
			}
			results = append(results, result)
			observed = append(observed, current)
		}
	}
	summary := health.Summary(results)
	application.Status.Resources = summary
	evident := r.diagnose(ctx, tenant, application, results, observed, summary.Degraded > 0, previousHealth)
	if summary.Healthy == summary.Total {
		return corev1alpha1.HealthStateHealthy, "", nil
	}
	state := corev1alpha1.HealthStateProgressing
	if summary.Degraded > 0 || evident {
		state = corev1alpha1.HealthStateDegraded
	}
	message := fmt.Sprintf("%d of %d resources are not healthy", summary.Total-summary.Healthy, summary.Total)
	if len(application.Status.Diagnosis) > 0 {
		cause := application.Status.Diagnosis[0]
		message = fmt.Sprintf("%s %s/%s: %s", cause.Resource.Kind, cause.Resource.Namespace, cause.Resource.Name, cause.Reason)
	}
	return state, message, nil
}

// markFinishedRolloutUnhealthy records that a finished rollout's resources are
// no longer Healthy: the Application's health and a false Ready condition
// that keeps saying when the desired commit is held, with a HealthDegraded
// Event when it newly becomes Degraded. It sends no notification.
func (r *ApplicationReconciler) markFinishedRolloutUnhealthy(ctx context.Context, application *corev1alpha1.Application, state corev1alpha1.HealthState, message string, previousHealth corev1alpha1.HealthState) {
	application.Status.Health.State, application.Status.State = state, state
	reason := string(state)
	if hold := holdFrom(ctx); hold != nil {
		reason, message = "RolledBack", message+"; "+holdMessage(hold.manual)
	}
	setReady(application, metav1.ConditionFalse, reason, message)
	if state == corev1alpha1.HealthStateDegraded && previousHealth != corev1alpha1.HealthStateDegraded {
		r.event(application, corev1.EventTypeWarning, "HealthDegraded", "Application became Degraded after its rollout: "+message)
	}
}

// keepFinishedRollout persists an Application whose Revision's rollout
// finished, with the Revision keeping its finished phase, and checks back
// after healthRecheck.
func (r *ApplicationReconciler) keepFinishedRollout(ctx context.Context, application *corev1alpha1.Application, revision *corev1alpha1.Revision, previousPhase corev1alpha1.RevisionPhase) (ctrl.Result, error) {
	revision.Status.Phase = finishedPhase(previousPhase)
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: r.healthRecheck()}, r.updateApplicationStatus(ctx, application)
}
