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

// Evaluate returns health for MVP Kubernetes resource kinds.
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
	case "Service", "ConfigMap", "Secret", "PersistentVolumeClaim":
		return result, nil
	default:
		return Result{Resource: id, State: corev1alpha1.HealthStateUnknown, Reason: "UnsupportedKind", Message: "no built-in evaluator for kind"}, nil
	}
	return result, nil
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
