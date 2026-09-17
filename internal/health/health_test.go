package health

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEvaluateMVPResources(t *testing.T) {
	ready := deployment("api", 3, 3)
	got, err := Evaluate(ready)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != corev1alpha1.HealthStateHealthy {
		t.Fatalf("ready deployment = %s", got.State)
	}
	progressing, err := Evaluate(deployment("api", 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if progressing.State != corev1alpha1.HealthStateProgressing || progressing.Reason != "ReplicasUnavailable" {
		t.Fatalf("progressing deployment = %#v", progressing)
	}
	failedPod := obj("v1", "Pod", "payments", "api-1")
	_ = unstructured.SetNestedField(failedPod.Object, "Failed", "status", "phase")
	degraded, err := Evaluate(failedPod)
	if err != nil {
		t.Fatal(err)
	}
	if degraded.State != corev1alpha1.HealthStateDegraded {
		t.Fatalf("failed pod = %#v", degraded)
	}
}

func TestObserveStopsOnHealthyAndTimeout(t *testing.T) {
	calls := 0
	results, err := Observe(context.Background(), time.Millisecond, time.Second, func(context.Context) ([]unstructured.Unstructured, error) {
		calls++
		if calls == 1 {
			return []unstructured.Unstructured{deployment("api", 2, 1)}, nil
		}
		return []unstructured.Unstructured{deployment("api", 2, 2)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].State != corev1alpha1.HealthStateHealthy || calls != 2 {
		t.Fatalf("unexpected observe result calls=%d results=%#v", calls, results)
	}
	_, err = Observe(context.Background(), time.Millisecond, time.Millisecond, func(context.Context) ([]unstructured.Unstructured, error) {
		return []unstructured.Unstructured{deployment("api", 2, 1)}, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func deployment(name string, replicas, available int64) unstructured.Unstructured {
	obj := obj("apps/v1", "Deployment", "payments", name)
	obj.SetGeneration(1)
	_ = unstructured.SetNestedField(obj.Object, replicas, "spec", "replicas")
	_ = unstructured.SetNestedField(obj.Object, available, "status", "availableReplicas")
	_ = unstructured.SetNestedField(obj.Object, int64(1), "status", "observedGeneration")
	return obj
}

func obj(apiVersion, kind, namespace, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]any{"namespace": namespace, "name": name}}}
}
