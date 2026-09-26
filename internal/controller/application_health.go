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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/health"
	"github.com/azrtydxb/kuvryn-sync/internal/ordering"
)

// steadyStateRecheck is how soon an Application whose finished rollout is no
// longer healthy is checked again, besides the watches on its workloads.
const steadyStateRecheck = 30 * time.Second

// reportSteadyStateHealth evaluates the live health of a finished rollout's
// resources, which can stop being available while nothing changes in Git.
// When any is not Healthy it records the Application as Degraded, if a
// resource is Degraded or diagnosis finds evidence such as a crash loop, or
// as Progressing otherwise, and reports unhealthy. The Revision keeps its
// finished phase: its rollout completed, and the failure policy acts only on
// rollouts. When health cannot be evaluated the Application keeps the health
// its rollout ended with, as before.
func (r *ApplicationReconciler) reportSteadyStateHealth(ctx context.Context, tenant client.Client, application *corev1alpha1.Application, revision *corev1alpha1.Revision, previousPhase corev1alpha1.RevisionPhase, rendered []unstructured.Unstructured, previousHealth corev1alpha1.HealthState) (ctrl.Result, bool, error) {
	log := logf.FromContext(ctx)
	evaluator, err := r.healthEvaluator(ctx, application)
	if err != nil {
		log.Error(err, "Could not read HealthChecks for a finished rollout", "application", application.Name, "namespace", application.Namespace)
		return ctrl.Result{}, false, nil
	}
	var objects []unstructured.Unstructured
	for _, group := range ordering.Groups(rendered) {
		if group.Stage == ordering.StageSync {
			objects = append(objects, group.Objects...)
		}
	}
	results, observed, err := groupHealth(ctx, tenant, evaluator, objects, false)
	if err != nil {
		log.Error(err, "Could not read managed resources of a finished rollout", "application", application.Name, "namespace", application.Namespace)
		return ctrl.Result{}, false, nil
	}
	summary := health.Summary(results)
	if summary.Healthy == summary.Total {
		return ctrl.Result{}, false, nil
	}
	evident := r.diagnose(ctx, tenant, application, results, observed, summary.Degraded > 0, previousHealth)
	state := corev1alpha1.HealthStateProgressing
	if summary.Degraded > 0 || evident {
		state = corev1alpha1.HealthStateDegraded
	}
	message := fmt.Sprintf("%d of %d resources are not healthy", summary.Total-summary.Healthy, summary.Total)
	if len(application.Status.Diagnosis) > 0 {
		cause := application.Status.Diagnosis[0]
		message = fmt.Sprintf("%s %s: %s", cause.Resource.Kind, cause.Resource.Name, cause.Reason)
	}
	application.Status.Resources = summary
	application.Status.Sync.State = corev1alpha1.SyncStateSynced
	application.Status.Health.State, application.Status.State = state, state
	setReady(application, metav1.ConditionFalse, string(state), message)
	if state == corev1alpha1.HealthStateDegraded && previousHealth != corev1alpha1.HealthStateDegraded {
		r.event(application, corev1.EventTypeWarning, "HealthDegraded", "Application became Degraded after its rollout: "+message)
	}
	revision.Status.Phase = finishedPhase(previousPhase)
	if err := r.updateRevisionStatus(ctx, revision); err != nil {
		return ctrl.Result{}, true, err
	}
	if err := r.updateApplicationStatus(ctx, application); err != nil {
		return ctrl.Result{}, true, err
	}
	log.Info("Application's finished rollout is not healthy", "application", application.Name, "namespace", application.Namespace, "health", state)
	return ctrl.Result{RequeueAfter: steadyStateRecheck}, true, nil
}
