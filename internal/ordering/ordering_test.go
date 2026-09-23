package ordering

import (
	"reflect"
	"strconv"
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
		annotated("Deployment", "api", map[string]string{"solder.io/sync-wave": "1"}),
		annotated("Job", "smoke", map[string]string{"solder.io/hook": "post-sync"}),
		annotated("ConfigMap", "api-config", map[string]string{"solder.io/sync-wave": "1"}),
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
