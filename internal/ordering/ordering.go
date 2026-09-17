package ordering

import (
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Apply returns deterministic dependency-aware apply order for common Kubernetes kinds.
func Apply(objects []unstructured.Unstructured) []unstructured.Unstructured {
	out := copyObjects(objects)
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := layer(out[i]), layer(out[j])
		if li != lj {
			return li < lj
		}
		return identity(out[i]) < identity(out[j])
	})
	return out
}

// Prune returns reverse dependency order for safe deletion.
func Prune(objects []unstructured.Unstructured) []unstructured.Unstructured {
	out := Apply(objects)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func copyObjects(objects []unstructured.Unstructured) []unstructured.Unstructured {
	out := make([]unstructured.Unstructured, len(objects))
	copy(out, objects)
	return out
}

func layer(obj unstructured.Unstructured) int {
	switch obj.GetKind() {
	case "Namespace", "CustomResourceDefinition":
		return 0
	case "ServiceAccount", "Role", "ClusterRole", "RoleBinding", "ClusterRoleBinding", "ConfigMap", "Secret", "PersistentVolumeClaim":
		return 10
	case "Service":
		return 20
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob":
		return 30
	case "Ingress", "HTTPRoute", "GRPCRoute":
		return 40
	default:
		return 50
	}
}

func identity(obj unstructured.Unstructured) string {
	return obj.GetAPIVersion() + "/" + obj.GetKind() + "/" + obj.GetNamespace() + "/" + obj.GetName()
}
