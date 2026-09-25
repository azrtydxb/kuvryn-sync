package ordering

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestApplyOrdersDependenciesBeforeWorkloads(t *testing.T) {
	ordered := Apply([]unstructured.Unstructured{
		obj("apps/v1", "Deployment", "payments", "api"),
		obj("v1", "Service", "payments", "api"),
		obj("v1", "Namespace", "", "payments"),
		obj("v1", "ConfigMap", "payments", "settings"),
	})
	got := kinds(ordered)
	want := []string{"Namespace", "ConfigMap", "Service", "Deployment"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestPruneOrdersReverseDependencies(t *testing.T) {
	ordered := Prune([]unstructured.Unstructured{
		obj("apps/v1", "Deployment", "payments", "api"),
		obj("v1", "Service", "payments", "api"),
		obj("v1", "Namespace", "", "payments"),
	})
	got := kinds(ordered)
	want := []string{"Deployment", "Service", "Namespace"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func obj(apiVersion, kind, namespace, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]any{"namespace": namespace, "name": name}}}
}

func kinds(objects []unstructured.Unstructured) []string {
	out := make([]string, len(objects))
	for i, obj := range objects {
		out[i] = obj.GetKind()
	}
	return out
}

func annotated(kind, name string, annotations map[string]string) unstructured.Unstructured {
	obj := unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": kind, "metadata": map[string]any{"name": name}}}
	obj.SetAnnotations(annotations)
	return obj
}

func TestGroupsOrderHooksWavesAndKinds(t *testing.T) {
	objects := []unstructured.Unstructured{
		annotated("Deployment", "api", map[string]string{"sync.kuvryn.io/sync-wave": "1"}),
		annotated("Job", "smoke", map[string]string{"sync.kuvryn.io/hook": "post-sync"}),
		annotated("ConfigMap", "api-config", map[string]string{"sync.kuvryn.io/sync-wave": "1"}),
		annotated("Service", "db", nil),
		annotated("Job", "migrate", map[string]string{"helm.sh/hook": "pre-upgrade,pre-install"}),
		annotated("CustomResourceDefinition", "widgets", map[string]string{"argocd.argoproj.io/sync-wave": "-1"}),
		annotated("Pod", "helm-test", map[string]string{"helm.sh/hook": "test"}),
	}
	groups := Groups(objects)
	got := make([][]string, 0, len(groups))
	for _, group := range groups {
		names := []string{group.Stage + ":" + strconv.Itoa(group.Wave)}
		for _, obj := range group.Objects {
			names = append(names, obj.GetName())
		}
		got = append(got, names)
	}
	want := [][]string{
		{"PreSync:0", "migrate"},
		{"Sync:-1", "widgets"},
		{"Sync:0", "db"},
		{"Sync:1", "api-config", "api"},
		{"PostSync:0", "smoke"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
}

func TestHookMapsEveryKnownAnnotationValue(t *testing.T) {
	cases := []struct {
		annotations map[string]string
		want        string
	}{
		{nil, ""},
		{map[string]string{"sync.kuvryn.io/hook": "pre-sync"}, StagePreSync},
		{map[string]string{"sync.kuvryn.io/hook": "Post-Sync"}, StagePostSync},
		{map[string]string{"sync.kuvryn.io/hook": "skip"}, StageSkip},
		{map[string]string{"argocd.argoproj.io/hook": "PreSync"}, StagePreSync},
		{map[string]string{"argocd.argoproj.io/hook": "PostSync"}, StagePostSync},
		{map[string]string{"argocd.argoproj.io/hook": "Sync"}, ""},
		{map[string]string{"argocd.argoproj.io/hook": "Skip"}, StageSkip},
		{map[string]string{"argocd.argoproj.io/hook": "SyncFail"}, StageSkip},
		{map[string]string{"argocd.argoproj.io/hook": "PreDelete"}, StageSkip},
		{map[string]string{"argocd.argoproj.io/hook": "PostDelete"}, StageSkip},
		{map[string]string{"argocd.argoproj.io/hook": "PreSync,SyncFail"}, StagePreSync},
		{map[string]string{"argocd.argoproj.io/hook": "PreSync,Skip"}, StageSkip},
		{map[string]string{"helm.sh/hook": "pre-install"}, StagePreSync},
		{map[string]string{"helm.sh/hook": "pre-upgrade"}, StagePreSync},
		{map[string]string{"helm.sh/hook": "post-install"}, StagePostSync},
		{map[string]string{"helm.sh/hook": "post-upgrade"}, StagePostSync},
		{map[string]string{"helm.sh/hook": "pre-delete"}, StageSkip},
		{map[string]string{"helm.sh/hook": "post-delete"}, StageSkip},
		{map[string]string{"helm.sh/hook": "pre-rollback"}, StageSkip},
		{map[string]string{"helm.sh/hook": "post-rollback"}, StageSkip},
		{map[string]string{"helm.sh/hook": "test"}, StageSkip},
		{map[string]string{"helm.sh/hook": "test-success"}, StageSkip},
		{map[string]string{"helm.sh/hook": "test-failure"}, StageSkip},
		{map[string]string{"helm.sh/hook": "post-delete, pre-install"}, StagePreSync},
		{map[string]string{"helm.sh/hook": "pre-rollback,post-upgrade"}, StagePostSync},
		{map[string]string{"sync.kuvryn.io/hook": "post-sync", "helm.sh/hook": "pre-install"}, StagePostSync},
		{map[string]string{"argocd.argoproj.io/hook": "Skip", "helm.sh/hook": "pre-install"}, StageSkip},
	}
	for _, tc := range cases {
		obj := annotated("Job", "hook", tc.annotations)
		if got := Hook(obj); got != tc.want {
			t.Errorf("Hook(%v) = %q, want %q", tc.annotations, got, tc.want)
		}
		if err := ValidateHooks([]unstructured.Unstructured{obj}); err != nil {
			t.Errorf("ValidateHooks(%v) = %v, want nil", tc.annotations, err)
		}
	}
}

func TestValidateHooksRefusesUnknownValues(t *testing.T) {
	for _, annotations := range []map[string]string{
		{"sync.kuvryn.io/hook": "pre-install"},
		{"sync.kuvryn.io/hook": ""},
		{"argocd.argoproj.io/hook": "presync"},
	} {
		obj := annotated("Job", "migrate", annotations)
		err := ValidateHooks([]unstructured.Unstructured{annotated("ConfigMap", "ok", nil), obj})
		if err == nil || !strings.Contains(err.Error(), "Job migrate") {
			t.Errorf("ValidateHooks(%v) = %v, want an error naming Job migrate", annotations, err)
		}
		if got := Hook(obj); got != StageSkip {
			t.Errorf("Hook(%v) = %q, want %q so it is never applied", annotations, got, StageSkip)
		}
	}
}

func TestGroupsDropHooksThatNeverRun(t *testing.T) {
	groups := Groups([]unstructured.Unstructured{
		annotated("ConfigMap", "app", nil),
		annotated("Job", "cleanup", map[string]string{"helm.sh/hook": "pre-delete"}),
		annotated("Job", "undo", map[string]string{"helm.sh/hook": "post-rollback"}),
		annotated("Job", "ignored", map[string]string{"argocd.argoproj.io/hook": "Skip"}),
	})
	if len(groups) != 1 || len(groups[0].Objects) != 1 || groups[0].Objects[0].GetName() != "app" {
		t.Fatalf("groups = %v, want only the ConfigMap in the sync stage", groups)
	}
}
