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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/ops"
)

// The reconcile duration must cover one reconcile, not the time since the
// reconciler was built.
func TestReconcileDurationIsMeasuredPerReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	const namespace = "reconcile-duration"
	r := &ApplicationReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}
	idle := 200 * time.Millisecond
	time.Sleep(idle)

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: "missing"}}); err != nil {
		t.Fatal(err)
	}

	histogram := ops.ReconcileDurationSeconds.WithLabelValues(namespace, "unknown", "unknown", "unknown", "success").(prometheus.Metric)
	sample := &dto.Metric{}
	if err := histogram.Write(sample); err != nil {
		t.Fatal(err)
	}
	if got := sample.GetHistogram().GetSampleCount(); got != 1 {
		t.Fatalf("observed %d reconciles, want 1", got)
	}
	if got := time.Duration(sample.GetHistogram().GetSampleSum() * float64(time.Second)); got >= idle {
		t.Fatalf("observed reconcile duration %s includes the %s before the reconcile", got, idle)
	}
}
