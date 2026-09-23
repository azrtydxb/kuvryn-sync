package health

import (
	context "context"
	"fmt"
	"time"

	corev1alpha1 "github.com/azrtydxb/solder/api/v1alpha1"
	"github.com/azrtydxb/solder/internal/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Result is a deterministic health verdict for a resource.
type Result struct {
	Resource resource.ID              `json:"resource"`
	State    corev1alpha1.HealthState `json:"state"`
	Reason   string                   `json:"reason,omitempty"`
	Message  string                   `json:"message,omitempty"`
}

// Evaluate returns health for a live Kubernetes object: built-in workload
// kinds use dedicated rules and every other kind follows kstatus conventions.
func Evaluate(obj unstructured.Unstructured) (Result, error) {
	id, err := resource.FromObject(obj)
	if err != nil {
		return Result{}, err
	}
	result := Result{Resource: id, State: corev1alpha1.HealthStateHealthy, Reason: "Ready"}
	switch obj.GetKind() {
	case "Deployment", "StatefulSet", "DaemonSet":
		desired := int64OrDefault(obj, 1, "spec", "replicas")
		available := int64OrDefault(obj, 0, "status", "availableReplicas")
		observed := int64OrDefault(obj, 0, "status", "observedGeneration")
		generation := obj.GetGeneration()
		if generation > 0 && observed < generation {
			return progressing(result, "GenerationPending", "controller has not observed latest generation"), nil
		}
		if available < desired {
			return progressing(result, "ReplicasUnavailable", fmt.Sprintf("%d/%d replicas available", available, desired)), nil
		}
	case "Pod":
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		switch phase {
		case "Running", "Succeeded":
			return result, nil
		case "Failed":
			return Result{Resource: id, State: corev1alpha1.HealthStateDegraded, Reason: "PodFailed", Message: "pod phase is Failed"}, nil
		default:
			return progressing(result, "PodPending", "pod is not running"), nil
		}
	case "Job":
		conditions := statusConditions(obj)
		if condition, ok := conditions["Failed"]; ok && condition.status == "True" {
			return Result{Resource: id, State: corev1alpha1.HealthStateDegraded, Reason: "JobFailed", Message: condition.message}, nil
		}
		if condition, ok := conditions["Complete"]; ok && condition.status == "True" {
			return result, nil
		}
		return progressing(result, "JobRunning", "job has not completed"), nil
	default:
		return generic(result, obj), nil
	}
	return result, nil
}

// generic applies kstatus conventions: an unobserved generation or a
// Reconciling condition is in progress, a Stalled condition is degraded, a
// Ready condition decides otherwise, and an object without status is healthy.
func generic(result Result, obj unstructured.Unstructured) Result {
	if observed, found, _ := unstructured.NestedInt64(obj.Object, "status", "observedGeneration"); found && observed < obj.GetGeneration() {
		return progressing(result, "GenerationPending", "controller has not observed latest generation")
	}
	conditions := statusConditions(obj)
	if condition, ok := conditions["Stalled"]; ok && condition.status == "True" {
		result.State = corev1alpha1.HealthStateDegraded
		result.Reason = "Stalled"
		result.Message = condition.message
		return result
	}
	if condition, ok := conditions["Reconciling"]; ok && condition.status == "True" {
		return progressing(result, "Reconciling", condition.message)
	}
	if condition, ok := conditions["Ready"]; ok && condition.status != "True" {
		return progressing(result, "NotReady", condition.message)
	}
	return result
}

type condition struct {
	status  string
	message string
}

func statusConditions(obj unstructured.Unstructured) map[string]condition {
	out := map[string]condition{}
	raw, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := entry["type"].(string)
		status, _ := entry["status"].(string)
		message, _ := entry["message"].(string)
		if kind != "" {
			out[kind] = condition{status: status, message: message}
		}
	}
	return out
}

// Summary counts health states.
func Summary(results []Result) corev1alpha1.ResourceHealthSummary {
	out := corev1alpha1.ResourceHealthSummary{Total: int32(len(results))}
	for _, result := range results {
		switch result.State {
		case corev1alpha1.HealthStateHealthy:
			out.Healthy++
		case corev1alpha1.HealthStateProgressing:
			out.Progressing++
		case corev1alpha1.HealthStateDegraded:
			out.Degraded++
		default:
			out.Unknown++
		}
	}
	return out
}

// Observe evaluates snapshots until all are healthy, any is degraded, timeout fires, or context is cancelled.
func Observe(ctx context.Context, interval, timeout time.Duration, snapshots func(context.Context) ([]unstructured.Unstructured, error)) ([]Result, error) {
	if interval <= 0 {
		interval = 10 * time.Millisecond
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	for {
		objects, err := snapshots(ctx)
		if err != nil {
			return nil, err
		}
		results := make([]Result, 0, len(objects))
		for _, obj := range objects {
			result, err := Evaluate(obj)
			if err != nil {
				return nil, err
			}
			results = append(results, result)
		}
		if terminal(results) {
			return results, nil
		}
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func terminal(results []Result) bool {
	allHealthy := len(results) > 0
	for _, result := range results {
		if result.State == corev1alpha1.HealthStateDegraded {
			return true
		}
		if result.State != corev1alpha1.HealthStateHealthy {
			allHealthy = false
		}
	}
	return allHealthy
}

func progressing(result Result, reason, message string) Result {
	result.State = corev1alpha1.HealthStateProgressing
	result.Reason = reason
	result.Message = message
	return result
}

func int64OrDefault(obj unstructured.Unstructured, fallback int64, fields ...string) int64 {
	value, ok, _ := unstructured.NestedInt64(obj.Object, fields...)
	if !ok {
		return fallback
	}
	return value
}
