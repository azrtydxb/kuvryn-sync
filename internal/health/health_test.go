package health

import (
	"testing"

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

func TestEvaluateFollowsKstatusForOtherKinds(t *testing.T) {
	withConditions := func(generation, observed int64, conditions ...map[string]any) unstructured.Unstructured {
		o := obj("cert-manager.io/v1", "Certificate", "payments", "tls")
		o.SetGeneration(generation)
		_ = unstructured.SetNestedField(o.Object, observed, "status", "observedGeneration")
		list := make([]any, 0, len(conditions))
		for _, c := range conditions {
			list = append(list, c)
		}
		_ = unstructured.SetNestedSlice(o.Object, list, "status", "conditions")
		return o
	}
	cond := func(kind, status string) map[string]any {
		return map[string]any{"type": kind, "status": status, "message": kind + " is " + status}
	}
	cases := []struct {
		name   string
		obj    unstructured.Unstructured
		state  corev1alpha1.HealthState
		reason string
	}{
		{"current", withConditions(2, 2, cond("Ready", "True")), corev1alpha1.HealthStateHealthy, "Ready"},
		{"generation not observed", withConditions(3, 2, cond("Ready", "True")), corev1alpha1.HealthStateProgressing, "GenerationPending"},
		{"reconciling", withConditions(2, 2, cond("Reconciling", "True"), cond("Ready", "True")), corev1alpha1.HealthStateProgressing, "Reconciling"},
		{"not ready", withConditions(2, 2, cond("Ready", "False")), corev1alpha1.HealthStateProgressing, "NotReady"},
		{"stalled", withConditions(2, 2, cond("Stalled", "True"), cond("Ready", "False")), corev1alpha1.HealthStateDegraded, "Stalled"},
		{"no status", obj("example.com/v1", "Widget", "payments", "gear"), corev1alpha1.HealthStateHealthy, "Ready"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Evaluate(tc.obj)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.state || got.Reason != tc.reason {
				t.Fatalf("got %s/%s, want %s/%s", got.State, got.Reason, tc.state, tc.reason)
			}
		})
	}
}

func TestEvaluateJobs(t *testing.T) {
	job := func(conditionType string) unstructured.Unstructured {
		o := obj("batch/v1", "Job", "payments", "migrate")
		if conditionType != "" {
			_ = unstructured.SetNestedSlice(o.Object, []any{map[string]any{"type": conditionType, "status": "True"}}, "status", "conditions")
		}
		return o
	}
	for conditionType, want := range map[string]corev1alpha1.HealthState{
		"":         corev1alpha1.HealthStateProgressing,
		"Complete": corev1alpha1.HealthStateHealthy,
		"Failed":   corev1alpha1.HealthStateDegraded,
	} {
		got, err := Evaluate(job(conditionType))
		if err != nil {
			t.Fatal(err)
		}
		if got.State != want {
			t.Fatalf("job with %q condition = %s, want %s", conditionType, got.State, want)
		}
	}
}

func TestDeploymentPastItsProgressDeadlineIsDegraded(t *testing.T) {
	stuck := deployment("api", 1, 0)
	_ = unstructured.SetNestedSlice(stuck.Object, []any{map[string]any{
		"type": "Progressing", "status": "False", "reason": "ProgressDeadlineExceeded", "message": `ReplicaSet "api-1" has timed out progressing.`,
	}}, "status", "conditions")
	got, err := Evaluate(stuck)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != corev1alpha1.HealthStateDegraded || got.Reason != "ProgressDeadlineExceeded" || got.Message == "" {
		t.Fatalf("stuck deployment = %#v", got)
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
