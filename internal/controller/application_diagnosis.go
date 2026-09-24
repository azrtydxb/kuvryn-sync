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

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/diagnosis"
	"github.com/azrtydxb/solder/internal/graph"
	"github.com/azrtydxb/solder/internal/health"
)

// diagnose records on the Application why its managed objects are not
// Healthy, and clears the diagnosis once they are. degraded is whether the
// Application is being marked Degraded. A Diagnosed Event names the first
// cause whenever the set of causes changes, unless every cause is only a
// managed object's own health verdict during a rollout that is not failing.
func (r *ApplicationReconciler) diagnose(ctx context.Context, tenant client.Reader, application *corev1alpha1.Application, results []health.Result, observed []unstructured.Unstructured, degraded bool) {
	if !slices.ContainsFunc(results, func(result health.Result) bool { return result.State != corev1alpha1.HealthStateHealthy }) {
		application.Status.Diagnosis = nil
		return
	}
	g, objects := graph.Collect(ctx, tenant, destinationNamespace(application), observed)
	causes := diagnosis.Build(diagnosis.Input{Results: results, Graph: g, Objects: objects})
	next := diagnosis.Status(causes)
	changed := !sameCauses(application.Status.Diagnosis, next)
	application.Status.Diagnosis = next
	if !changed || len(causes) == 0 {
		return
	}
	evident := slices.ContainsFunc(causes, func(cause diagnosis.Cause) bool { return !cause.Fallback })
	if !evident && !degraded {
		return
	}
	first := next[0]
	message := fmt.Sprintf("%s %s: %s", first.Resource.Kind, refName(first.Resource), first.Reason)
	if first.Message != "" {
		message += ": " + first.Message
	}
	if len(next) > 1 {
		message += fmt.Sprintf(" (and %d more)", len(next)-1)
	}
	r.event(application, corev1.EventTypeWarning, "Diagnosed", message)
}

// sameCauses compares the root resources and reasons of two diagnoses;
// messages carry counters and back-off times that change on their own.
func sameCauses(a, b []corev1alpha1.DiagnosisCause) bool {
	return slices.EqualFunc(a, b, func(x, y corev1alpha1.DiagnosisCause) bool {
		return x.Resource == y.Resource && x.Reason == y.Reason
	})
}

func refName(ref corev1alpha1.ResourceRef) string {
	if ref.Namespace == "" {
		return ref.Name
	}
	return ref.Namespace + "/" + ref.Name
}

func destinationNamespace(application *corev1alpha1.Application) string {
	if application.Spec.Destination.Namespace != "" {
		return application.Spec.Destination.Namespace
	}
	return application.Namespace
}
