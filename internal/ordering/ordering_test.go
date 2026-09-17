package ordering

import (
	"reflect"
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
