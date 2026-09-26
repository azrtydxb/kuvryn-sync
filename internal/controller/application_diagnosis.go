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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/diagnosis"
	"github.com/azrtydxb/kuvryn-sync/internal/graph"
	"github.com/azrtydxb/kuvryn-sync/internal/health"
)

// diagnose records on the Application why its managed objects are not
// Healthy, and clears the diagnosis once they are. degraded is whether the
// Application is being marked Degraded, and previousHealth the health
// persisted before this reconcile. A Diagnosed Event names the first cause
// when the set of causes changes or the Application newly becomes Degraded,
// unless every cause is only a managed object's own health verdict during a
// rollout that is not failing. It returns whether any cause has evidence
// beyond such a verdict, such as a crash loop or a missing Secret.
func (r *ApplicationReconciler) diagnose(ctx context.Context, tenant client.Reader, application *corev1alpha1.Application, results []health.Result, observed []unstructured.Unstructured, degraded bool, previousHealth corev1alpha1.HealthState) (evident bool) {
	if !slices.ContainsFunc(results, func(result health.Result) bool { return result.State != corev1alpha1.HealthStateHealthy }) {
		application.Status.Diagnosis = nil
		return false
	}
	g, objects := graph.Collect(ctx, tenant, application.DestinationNamespace(), observed)
	causes := diagnosis.Build(ctx, diagnosis.Input{Results: results, Graph: g, Objects: objects})
	next := diagnosis.Status(causes)
	changed := !sameCauses(application.Status.Diagnosis, next)
	newlyDegraded := degraded && previousHealth != corev1alpha1.HealthStateDegraded
	application.Status.Diagnosis = next
	evident = slices.ContainsFunc(causes, func(cause diagnosis.Cause) bool { return !cause.Fallback })
	if len(causes) == 0 || (!changed && !newlyDegraded) {
		return evident
	}
	if !evident && !degraded {
		return false
	}
	first := causes[0]
	message := fmt.Sprintf("%s %s: %s", first.Resource.Kind, first.Resource.QualifiedName(), first.Reason)
	if first.Message != "" {
		message += ": " + first.Message
	}
	if len(next) > 1 {
		message += fmt.Sprintf(" (and %d more)", len(next)-1)
	}
	r.event(application, corev1.EventTypeWarning, "Diagnosed", message)
	return evident
}

// sameCauses compares the root resources and reasons of two diagnoses;
// messages carry counters and back-off times that change on their own.
func sameCauses(a, b []corev1alpha1.DiagnosisCause) bool {
	return slices.EqualFunc(a, b, func(x, y corev1alpha1.DiagnosisCause) bool {
		return x.Resource == y.Resource && x.Reason == y.Reason
	})
}
