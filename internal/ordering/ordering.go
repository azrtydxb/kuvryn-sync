package ordering

import (
	"sort"
	"strconv"
	"strings"

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

// Sync stages, in the order they run.
const (
	StagePreSync  = "PreSync"
	StageSync     = "Sync"
	StagePostSync = "PostSync"
	// StageSkip marks Helm test hooks, which a deployment never applies.
	StageSkip = "Skip"
)

// Group is a set of objects applied together and waited on until healthy
// before the next group starts.
type Group struct {
	Stage   string
	Wave    int
	Objects []unstructured.Unstructured
}

// Hook reports the stage of a hook object, honouring Solder's annotation and
// the Helm and Argo CD equivalents, or "" for ordinary objects. Helm test
// hooks report StageSkip.
func Hook(obj unstructured.Unstructured) string {
	annotations := obj.GetAnnotations()
	switch strings.ToLower(annotations["solder.io/hook"]) {
	case "pre-sync":
		return StagePreSync
	case "post-sync":
		return StagePostSync
	}
	switch annotations["argocd.argoproj.io/hook"] {
	case "PreSync":
		return StagePreSync
	case "PostSync":
		return StagePostSync
	}
	for _, event := range strings.Split(annotations["helm.sh/hook"], ",") {
		switch strings.TrimSpace(event) {
		case "pre-install", "pre-upgrade":
			return StagePreSync
		case "post-install", "post-upgrade":
			return StagePostSync
		case "test", "test-success", "test-failure":
			return StageSkip
		}
	}
	return ""
}

// Wave reads the sync wave from Solder's or Argo CD's annotation; 0 when unset
// or invalid.
func Wave(obj unstructured.Unstructured) int {
	for _, key := range []string{"solder.io/sync-wave", "argocd.argoproj.io/sync-wave"} {
		if value, ok := obj.GetAnnotations()[key]; ok {
			if wave, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				return wave
			}
		}
	}
	return 0
}

// Groups splits objects into pre-sync hooks, then one group per sync wave in
// ascending order, then post-sync hooks, dropping Helm test hooks. Each group
// is in apply order.
func Groups(objects []unstructured.Unstructured) []Group {
	var pre, post []unstructured.Unstructured
	waves := map[int][]unstructured.Unstructured{}
	for _, obj := range objects {
		switch Hook(obj) {
		case StagePreSync:
			pre = append(pre, obj)
		case StagePostSync:
			post = append(post, obj)
		case StageSkip:
		default:
			waves[Wave(obj)] = append(waves[Wave(obj)], obj)
		}
	}
	groups := []Group{}
	if len(pre) > 0 {
		groups = append(groups, Group{Stage: StagePreSync, Objects: Apply(pre)})
	}
	numbers := make([]int, 0, len(waves))
	for wave := range waves {
		numbers = append(numbers, wave)
	}
	sort.Ints(numbers)
	for _, wave := range numbers {
		groups = append(groups, Group{Stage: StageSync, Wave: wave, Objects: Apply(waves[wave])})
	}
	if len(post) > 0 {
		groups = append(groups, Group{Stage: StagePostSync, Objects: Apply(post)})
	}
	return groups
}
