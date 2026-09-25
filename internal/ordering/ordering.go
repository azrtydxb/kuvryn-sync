package ordering

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Apply returns deterministic dependency-aware apply order for common Kubernetes kinds.
func Apply(objects []unstructured.Unstructured) []unstructured.Unstructured {
	out := slices.Clone(objects)
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
	slices.Reverse(out)
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
	// StageSkip marks hooks a deployment never applies: Helm test, delete and
	// rollback hooks, Argo CD Skip, SyncFail and delete hooks, and
	// sync.kuvryn.io/hook: skip.
	StageSkip = "Skip"
)

// Group is a set of objects applied together and waited on until healthy
// before the next group starts.
type Group struct {
	Stage   string
	Wave    int
	Objects []unstructured.Unstructured
}

// Hook annotations Solder honours, in order of precedence.
const (
	SolderHookAnnotation = "sync.kuvryn.io/hook"
	ArgoHookAnnotation   = "argocd.argoproj.io/hook"
	HelmHookAnnotation   = "helm.sh/hook"
)

// Hook reports the stage of a hook object, honouring Solder's annotation and
// the Helm and Argo CD equivalents, or "" for ordinary objects. Hooks a
// deployment never runs report StageSkip, and so does a Solder or Argo CD hook
// value Solder does not know; ValidateHooks refuses those before any apply.
func Hook(obj unstructured.Unstructured) string {
	stage, err := hookStage(obj.GetAnnotations())
	if err != nil {
		return StageSkip
	}
	return stage
}

// ValidateHooks refuses objects whose sync.kuvryn.io/hook or argocd.argoproj.io/hook
// value is unknown, rather than guessing when to apply them.
func ValidateHooks(objects []unstructured.Unstructured) error {
	for _, obj := range objects {
		if _, err := hookStage(obj.GetAnnotations()); err != nil {
			return fmt.Errorf("rendered resource %s/%s %s: %w", obj.GetAPIVersion(), obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

func hookStage(annotations map[string]string) (string, error) {
	if value, ok := annotations[SolderHookAnnotation]; ok {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "pre-sync":
			return StagePreSync, nil
		case "post-sync":
			return StagePostSync, nil
		case "skip":
			return StageSkip, nil
		}
		return "", fmt.Errorf("unknown %s value %q: use pre-sync, post-sync, or skip", SolderHookAnnotation, value)
	}
	if value, ok := annotations[ArgoHookAnnotation]; ok {
		return argoHookStage(value)
	}
	return helmHookStage(annotations[HelmHookAnnotation]), nil
}

// argoHookStage maps Argo CD hook types. Sync hooks run with ordinary objects;
// Skip, SyncFail and delete hooks never run during a deployment.
func argoHookStage(value string) (string, error) {
	stage, skip := "", false
	for _, event := range strings.Split(value, ",") {
		switch strings.TrimSpace(event) {
		case "PreSync":
			stage = firstStage(stage, StagePreSync)
		case "PostSync":
			stage = firstStage(stage, StagePostSync)
		case "Sync":
			stage = firstStage(stage, StageSync)
		case "Skip":
			return StageSkip, nil
		case "SyncFail", "PreDelete", "PostDelete":
			skip = true
		default:
			return "", fmt.Errorf("unknown %s value %q", ArgoHookAnnotation, value)
		}
	}
	switch {
	case stage == StageSync:
		return "", nil
	case stage != "":
		return stage, nil
	case skip:
		return StageSkip, nil
	}
	return "", nil
}

// helmHookStage maps Helm hook events. As in Helm, an object carrying the hook
// annotation is never an ordinary resource: without an install or upgrade
// event it never runs.
func helmHookStage(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	stage := StageSkip
	for _, event := range strings.Split(value, ",") {
		switch strings.ToLower(strings.TrimSpace(event)) {
		case "pre-install", "pre-upgrade":
			stage = firstStage(stage, StagePreSync)
		case "post-install", "post-upgrade":
			stage = firstStage(stage, StagePostSync)
		}
	}
	return stage
}

// firstStage keeps the first run stage found; StageSkip and "" are none.
func firstStage(current, next string) string {
	if current == "" || current == StageSkip {
		return next
	}
	return current
}

// Wave reads the sync wave from Solder's or Argo CD's annotation; 0 when unset
// or invalid.
func Wave(obj unstructured.Unstructured) int {
	for _, key := range []string{"sync.kuvryn.io/sync-wave", "argocd.argoproj.io/sync-wave"} {
		if value, ok := obj.GetAnnotations()[key]; ok {
			if wave, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				return wave
			}
		}
	}
	return 0
}

// Groups splits objects into pre-sync hooks, then one group per sync wave in
// ascending order, then post-sync hooks, dropping hooks that never run. Each group
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
	for _, wave := range slices.Sorted(maps.Keys(waves)) {
		groups = append(groups, Group{Stage: StageSync, Wave: wave, Objects: Apply(waves[wave])})
	}
	if len(post) > 0 {
		groups = append(groups, Group{Stage: StagePostSync, Objects: Apply(post)})
	}
	return groups
}
